package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

func NewPaymentRequestWriter(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        TopicPaymentRequests,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
}

type PaymentResultHandler interface {
	OnPaymentSucceeded(ctx context.Context, orderID uuid.UUID) error
	OnPaymentFailed(ctx context.Context, orderID uuid.UUID) error
}

func RunPaymentResultConsumer(ctx context.Context, brokers []string, h PaymentResultHandler) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:               brokers,
		GroupID:               "order-service-payments",
		Topic:                 TopicPaymentResults,
		MinBytes:              1,
		MaxBytes:              10 << 20,
		StartOffset:           kafka.FirstOffset,
		WatchPartitionChanges: true,
	})
	defer func() { _ = r.Close() }()

	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("kafka fetch", "err", err)
			time.Sleep(time.Second)
			continue
		}
		var env Envelope
		if err := json.Unmarshal(m.Value, &env); err != nil {
			slog.Error("json", "err", err)
		} else {
			switch env.Type {
			case "PaymentSucceeded":
				var p PaymentResultPayload
				if err := json.Unmarshal(env.Payload, &p); err != nil {
					slog.Error("payload", "err", err)
				} else if oid, err := uuid.Parse(p.OrderID); err == nil {
					if err := h.OnPaymentSucceeded(ctx, oid); err != nil {
						slog.Error("on succeeded", "err", err)
					}
				}
			case "PaymentFailed":
				var p PaymentResultPayload
				if err := json.Unmarshal(env.Payload, &p); err != nil {
					slog.Error("payload", "err", err)
				} else if oid, err := uuid.Parse(p.OrderID); err == nil {
					if err := h.OnPaymentFailed(ctx, oid); err != nil {
						slog.Error("on failed", "err", err)
					}
				}
			}
		}
		if err := r.CommitMessages(ctx, m); err != nil {
			slog.Error("commit", "err", err)
		}
	}
}

func MarshalPaymentRequest(orderID, paymentID uuid.UUID, amount string, idem uuid.UUID, method string) ([]byte, error) {
	env := Envelope{
		Type: "PaymentRequest",
		Payload: mustJSON(PaymentRequestPayload{
			OrderID:        orderID.String(),
			PaymentID:      paymentID.String(),
			Amount:         amount,
			IdempotencyKey: idem.String(),
			PaymentMethod:  method,
		}),
	}
	return json.Marshal(env)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
