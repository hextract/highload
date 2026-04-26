package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"highload/payment/internal/events"
	"highload/payment/internal/store"
)

type Bus struct {
	Store  *store.PaymentStore
	Writer *kafka.Writer
}

func RunPaymentRequests(ctx context.Context, brokers []string, bus *Bus) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:               brokers,
		GroupID:               "payment-service",
		Topic:                 events.TopicPaymentRequests,
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
		handle(ctx, bus, m.Value)
		if err := r.CommitMessages(ctx, m); err != nil {
			slog.Error("commit", "err", err)
		}
	}
}

func handle(ctx context.Context, bus *Bus, raw []byte) {
	var env events.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		slog.Error("json", "err", err)
		return
	}
	if env.Type != "PaymentRequest" {
		return
	}
	var req events.PaymentRequestPayload
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		slog.Error("payload", "err", err)
		return
	}
	pid, err := uuid.Parse(req.PaymentID)
	if err != nil {
		return
	}
	time.Sleep(80 * time.Millisecond)

	provID := uuid.New().String()
	oid, _, ok, err := bus.Store.MarkSucceeded(ctx, pid, "mock_"+provID)
	if err != nil {
		slog.Error("db", "err", err)
		return
	}
	if !ok {
		return
	}

	inner, err := json.Marshal(events.PaymentResultPayload{
		OrderID:   oid.String(),
		PaymentID: req.PaymentID,
	})
	if err != nil {
		slog.Error("marshal inner", "err", err)
		return
	}
	out, err := json.Marshal(events.Envelope{
		Type:    "PaymentSucceeded",
		Payload: inner,
	})
	if err != nil {
		slog.Error("marshal", "err", err)
		return
	}
	msg := kafka.Message{Key: []byte(oid.String()), Value: out}
	if err := bus.Writer.WriteMessages(ctx, msg); err != nil {
		slog.Error("kafka publish", "err", err)
	}
}
