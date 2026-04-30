package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderStore struct {
	pool *pgxpool.Pool
}

func NewOrderStore(pool *pgxpool.Pool) *OrderStore {
	return &OrderStore{pool: pool}
}

type CreateOrderInput struct {
	RestaurantID      uuid.UUID
	DeliveryText      string
	DeliveryLat       float64
	DeliveryLon       float64
	Total               float64
	Comment             *string
	EstimatedDelivery   time.Time
	Lines               []OrderLineInput
}

type OrderLineInput struct {
	MenuItemID uuid.UUID
	Name       string
	Qty        int
	Unit       float64
	Subtotal   float64
}

type CreateOrderResult struct {
	OrderID   uuid.UUID
	Estimated time.Time
	Items     []OrderLineOut
	Total     float64
}

type OrderLineOut struct {
	MenuItemID string
	Name       string
	Qty        int
	Unit       float64
	Subtotal   float64
}

func (s *OrderStore) CreateOrder(ctx context.Context, in CreateOrderInput) (*CreateOrderResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	oid := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO orders (id, restaurant_id, status, delivery_address, delivery_lat, delivery_lon, total_amount, comment, estimated_delivery_at)
		VALUES ($1, $2, 'created', $3, $4, $5, $6, $7, $8)`,
		oid, in.RestaurantID, in.DeliveryText, in.DeliveryLat, in.DeliveryLon, in.Total, in.Comment, in.EstimatedDelivery)
	if err != nil {
		return nil, err
	}
	if len(in.Lines) > 0 {
		batch := &pgx.Batch{}
		for _, ln := range in.Lines {
			batch.Queue(`
				INSERT INTO order_items (order_id, menu_item_id, name, quantity, unit_price, total_price)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				oid, ln.MenuItemID, ln.Name, ln.Qty, ln.Unit, ln.Subtotal)
		}
		br := tx.SendBatch(ctx, batch)
		for range in.Lines {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return nil, err
			}
		}
		if err := br.Close(); err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO saga_state (order_id, current_step, status, step_data)
		VALUES ($1, 'created', 'running', '{}')`, oid)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	out := &CreateOrderResult{OrderID: oid, Estimated: in.EstimatedDelivery, Total: in.Total}
	for _, ln := range in.Lines {
		out.Items = append(out.Items, OrderLineOut{
			MenuItemID: ln.MenuItemID.String(), Name: ln.Name, Qty: ln.Qty, Unit: ln.Unit, Subtotal: ln.Subtotal,
		})
	}
	return out, nil
}

type PayResult struct {
	Replay       bool
	OrderID      uuid.UUID
	PaymentID    uuid.UUID
	Total        float64
	Method       string
	Idempotency  uuid.UUID
}

func (s *OrderStore) Pay(ctx context.Context, orderID, idem uuid.UUID, paymentMethod string) (*PayResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	var total float64
	err = tx.QueryRow(ctx, `SELECT status, total_amount FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&status, &total)
	if err == pgx.ErrNoRows {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	if status == "paid" {
		return nil, ErrAlreadyPaid
	}

	var existingPID, payOwner uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id, order_id FROM payments WHERE idempotency_key = $1`, idem).Scan(&existingPID, &payOwner)
	if err == nil {
		if payOwner != orderID {
			return nil, ErrIdempotencyWrongOrder
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &PayResult{Replay: true, OrderID: orderID, PaymentID: existingPID}, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}
	if status != "created" {
		return nil, ErrOrderInvalidState
	}
	pid := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO payments (id, order_id, amount, status, payment_method, idempotency_key)
		VALUES ($1, $2, $3, 'pending', $4, $5)`,
		pid, orderID, total, paymentMethod, idem)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE orders SET status = 'payment_pending', updated_at = now() WHERE id = $1`, orderID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE saga_state SET current_step = 'payment_pending', updated_at = now() WHERE order_id = $1`, orderID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &PayResult{
		Replay: false, OrderID: orderID, PaymentID: pid, Total: total, Method: paymentMethod, Idempotency: idem,
	}, nil
}

func (s *OrderStore) Tracking(ctx context.Context, orderID uuid.UUID) (status string, createdAt, updatedAt time.Time, est *time.Time, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT status, created_at, updated_at, estimated_delivery_at FROM orders WHERE id = $1`, orderID).
		Scan(&status, &createdAt, &updatedAt, &est)
	return
}

func (s *OrderStore) OnPaymentSucceeded(ctx context.Context, orderID uuid.UUID) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE orders SET status = 'paid', updated_at = now() WHERE id = $1 AND status = 'payment_pending'`,
		orderID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE saga_state SET current_step = 'paid', status = 'running', updated_at = now() WHERE order_id = $1`, orderID)
	return err
}

func (s *OrderStore) OnPaymentFailed(ctx context.Context, orderID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE orders SET status = 'created', updated_at = now() WHERE id = $1 AND status = 'payment_pending'`, orderID)
	return err
}

var (
	ErrAlreadyPaid             = errors.New("already paid")
	ErrIdempotencyWrongOrder   = errors.New("idempotency key belongs to another order")
	ErrOrderInvalidState       = errors.New("invalid order state for payment")
)
