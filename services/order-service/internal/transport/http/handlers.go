package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/segmentio/kafka-go"

	"highload/order/internal/catalogclient"
	"highload/order/internal/events"
	"highload/order/internal/store"
)

type OrderHandler struct {
	Store   *store.OrderStore
	Catalog *catalogclient.Client
	Kafka   *kafka.Writer
}

type createOrderReq struct {
	RestaurantID string `json:"restaurant_id"`
	Items        []struct {
		MenuItemID string `json:"menu_item_id"`
		Quantity   int    `json:"quantity"`
	} `json:"items"`
	DeliveryAddress struct {
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		AddressText string  `json:"address_text"`
	} `json:"delivery_address"`
	Comment string `json:"comment"`
}

func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createOrderReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if len(body.Items) == 0 {
		http.Error(w, `{"error":"empty cart"}`, http.StatusBadRequest)
		return
	}
	rid, err := uuid.Parse(body.RestaurantID)
	if err != nil {
		http.Error(w, `{"error":"invalid restaurant"}`, http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	menu, code, err := h.Catalog.FetchMenu(ctx, rid)
	if err != nil {
		switch code {
		case http.StatusNotFound:
			http.Error(w, `{"error":"restaurant not found"}`, http.StatusNotFound)
		default:
			if code == 0 {
				slog.Error("catalog", "err", err)
				http.Error(w, `{"error":"catalog unavailable"}`, http.StatusServiceUnavailable)
			} else {
				slog.Error("catalog status", "code", code, "err", err)
				http.Error(w, `{"error":"catalog error"}`, http.StatusBadGateway)
			}
		}
		return
	}
	priceByID := map[string]struct {
		name  string
		price int
		ok    bool
	}{}
	for _, c := range menu.Categories {
		for _, it := range c.Items {
			priceByID[it.ID] = struct {
				name  string
				price int
				ok    bool
			}{it.Name, it.Price, it.Available}
		}
	}
	var lines []store.OrderLineInput
	var total float64
	for _, it := range body.Items {
		if it.Quantity <= 0 {
			http.Error(w, `{"error":"bad quantity"}`, http.StatusBadRequest)
			return
		}
		pi, ok := priceByID[it.MenuItemID]
		if !ok {
			http.Error(w, `{"error":"unknown menu item"}`, http.StatusNotFound)
			return
		}
		if !pi.ok {
			http.Error(w, `{"error":"item unavailable"}`, http.StatusConflict)
			return
		}
		mid, err := uuid.Parse(it.MenuItemID)
		if err != nil {
			http.Error(w, `{"error":"invalid item id"}`, http.StatusBadRequest)
			return
		}
		u := float64(pi.price)
		st := u * float64(it.Quantity)
		lines = append(lines, store.OrderLineInput{MenuItemID: mid, Name: pi.name, Qty: it.Quantity, Unit: u, Subtotal: st})
		total += st
	}
	var comment *string
	if body.Comment != "" {
		comment = &body.Comment
	}
	res, err := h.Store.CreateOrder(r.Context(), store.CreateOrderInput{
		RestaurantID:    rid,
		DeliveryText:    body.DeliveryAddress.AddressText,
		DeliveryLat:     body.DeliveryAddress.Lat,
		DeliveryLon:     body.DeliveryAddress.Lon,
		Total:           total,
		Comment:         comment,
		EstimatedDelivery: time.Now().UTC().Add(35 * time.Minute),
		Lines:           lines,
	})
	if err != nil {
		slog.Error("create order", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	type itemOut struct {
		MenuItemID string  `json:"menu_item_id"`
		Name       string  `json:"name"`
		Quantity   int     `json:"quantity"`
		UnitPrice  float64 `json:"unit_price"`
		TotalPrice float64 `json:"total_price"`
	}
	outItems := make([]itemOut, 0, len(res.Items))
	for _, ln := range res.Items {
		outItems = append(outItems, itemOut{ln.MenuItemID, ln.Name, ln.Qty, ln.Unit, ln.Subtotal})
	}
	out := map[string]any{
		"order_id":             res.OrderID.String(),
		"status":               "created",
		"items":                outItems,
		"total_amount":         res.Total,
		"estimated_delivery":   res.Estimated.UTC().Format(time.RFC3339),
		"created_at":           time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}

type payBody struct {
	PaymentMethod string `json:"payment_method"`
	CardToken     string `json:"card_token"`
}

func (h *OrderHandler) Pay(w http.ResponseWriter, r *http.Request) {
	idemStr := r.Header.Get("Idempotency-Key")
	if idemStr == "" {
		http.Error(w, `{"error":"Idempotency-Key required"}`, http.StatusBadRequest)
		return
	}
	idem, err := uuid.Parse(idemStr)
	if err != nil {
		http.Error(w, `{"error":"invalid Idempotency-Key"}`, http.StatusBadRequest)
		return
	}
	oid, err := uuid.Parse(chi.URLParam(r, "orderID"))
	if err != nil {
		http.Error(w, `{"error":"invalid order"}`, http.StatusBadRequest)
		return
	}
	var pb payBody
	_ = json.NewDecoder(r.Body).Decode(&pb)
	if pb.PaymentMethod == "" {
		pb.PaymentMethod = "card"
	}

	res, err := h.Store.Pay(r.Context(), oid, idem, pb.PaymentMethod)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrAlreadyPaid) {
		http.Error(w, `{"error":"already paid"}`, http.StatusConflict)
		return
	}
	if errors.Is(err, store.ErrIdempotencyWrongOrder) {
		http.Error(w, `{"error":"idempotency key belongs to another order"}`, http.StatusConflict)
		return
	}
	if errors.Is(err, store.ErrOrderInvalidState) {
		http.Error(w, `{"error":"invalid state"}`, http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if !res.Replay {
		raw, err := events.MarshalPaymentRequest(res.OrderID, res.PaymentID, fmt.Sprintf("%.2f", res.Total), res.Idempotency, res.Method)
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		msg := kafka.Message{Key: []byte(res.OrderID.String()), Value: raw}
		if err := h.Kafka.WriteMessages(r.Context(), msg); err != nil {
			slog.Error("kafka write", "err", err)
			http.Error(w, `{"error":"payment bus unavailable"}`, http.StatusServiceUnavailable)
			return
		}
	}
	body := map[string]any{
		"order_id":   oid.String(),
		"payment_id": res.PaymentID.String(),
		"status":     "payment_pending",
		"message":    "Оплата принята, ожидайте",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(body)
}

func (h *OrderHandler) Tracking(w http.ResponseWriter, r *http.Request) {
	oid, err := uuid.Parse(chi.URLParam(r, "orderID"))
	if err != nil {
		http.Error(w, `{"error":"invalid order"}`, http.StatusBadRequest)
		return
	}
	status, createdAt, updatedAt, est, err := h.Store.Tracking(r.Context(), oid)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	hist := []map[string]string{
		{"status": "created", "at": createdAt.UTC().Format(time.RFC3339)},
	}
	if status != "created" {
		hist = append(hist, map[string]string{"status": status, "at": updatedAt.UTC().Format(time.RFC3339)})
	}
	estStr := ""
	if est != nil {
		estStr = est.UTC().Format(time.RFC3339)
	}
	out := map[string]any{
		"order_id":             oid.String(),
		"status":               status,
		"status_history":       hist,
		"courier":              nil,
		"estimated_delivery": estStr,
		"updated_at":           updatedAt.UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
