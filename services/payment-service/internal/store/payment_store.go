package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PaymentStore struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *PaymentStore {
	return &PaymentStore{pool: pool}
}

func (s *PaymentStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PaymentStore) MarkSucceeded(ctx context.Context, paymentID uuid.UUID, providerTxID string) (orderID uuid.UUID, paymentIDOut uuid.UUID, ok bool, err error) {
	err = s.pool.QueryRow(ctx, `
		UPDATE payments SET status = 'succeeded', provider_tx_id = $2, updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING order_id, id`,
		paymentID, providerTxID).Scan(&orderID, &paymentIDOut)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, false, err
	}
	return orderID, paymentIDOut, true, nil
}

func (s *PaymentStore) PaymentOutcome(ctx context.Context, paymentID uuid.UUID) (orderID uuid.UUID, status string, found bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT order_id, status FROM payments WHERE id = $1`, paymentID).
		Scan(&orderID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", false, nil
	}
	if err != nil {
		return uuid.Nil, "", false, err
	}
	return orderID, status, true, nil
}
