package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"highload/payment/internal/events"
	"highload/payment/internal/store"
)

type Bus struct {
	Store    *store.PaymentStore
	Writer   *kafka.Writer
	DLQ      *kafka.Writer
	TopicSrc string
}

func RunPaymentRequests(ctx context.Context, brokers []string, bus *Bus) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:               brokers,
		GroupID:               "payment-service",
		Topic:                 events.TopicPaymentRequests,
		MinBytes:              1,
		MaxBytes:              1 << 20,
		SessionTimeout:        45 * time.Second,
		RebalanceTimeout:      70 * time.Second,
		HeartbeatInterval:     10 * time.Second,
		StartOffset:           kafka.FirstOffset,
		WatchPartitionChanges: true,
		Dialer: &kafka.Dialer{
			Timeout: 12 * time.Second,
		},
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
		decision := handleMessage(ctx, bus, m)
		switch decision {
		case decisionCommit:
			if err := r.CommitMessages(ctx, m); err != nil {
				slog.Error("commit", "err", err)
			}
		case decisionRetry:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

type decision int

const (
	decisionCommit decision = iota
	decisionRetry
)

func handleMessage(ctx context.Context, bus *Bus, m kafka.Message) decision {
	var env events.Envelope
	if err := json.Unmarshal(m.Value, &env); err != nil {
		if err := writeDLQ(ctx, bus, m, "json_envelope", err.Error()); err != nil {
			slog.Error("dlq", "err", err)
			return decisionRetry
		}
		return decisionCommit
	}
	if env.Type != "PaymentRequest" {
		if err := writeDLQ(ctx, bus, m, "unexpected_type", env.Type); err != nil {
			slog.Error("dlq", "err", err)
			return decisionRetry
		}
		return decisionCommit
	}
	var req events.PaymentRequestPayload
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		if err := writeDLQ(ctx, bus, m, "json_payload", err.Error()); err != nil {
			slog.Error("dlq", "err", err)
			return decisionRetry
		}
		return decisionCommit
	}
	pid, err := uuid.Parse(req.PaymentID)
	if err != nil {
		if err := writeDLQ(ctx, bus, m, "bad_payment_id", err.Error()); err != nil {
			slog.Error("dlq", "err", err)
			return decisionRetry
		}
		return decisionCommit
	}

	time.Sleep(80 * time.Millisecond)

	provID := uuid.New().String()
	oid, _, ok, err := bus.Store.MarkSucceeded(ctx, pid, "mock_"+provID)
	if err != nil {
		slog.Error("db", "err", err)
		return decisionRetry
	}
	if ok {
		if err := writePaymentSucceeded(ctx, bus.Writer, oid, req.PaymentID); err != nil {
			slog.Error("kafka publish", "err", err)
			return decisionRetry
		}
		return decisionCommit
	}

	orderID, status, found, err := bus.Store.PaymentOutcome(ctx, pid)
	if err != nil {
		slog.Error("db outcome", "err", err)
		return decisionRetry
	}
	if !found {
		return decisionCommit
	}
	switch status {
	case "succeeded":
		if err := writePaymentSucceeded(ctx, bus.Writer, orderID, req.PaymentID); err != nil {
			slog.Error("kafka publish replay", "err", err)
			return decisionRetry
		}
		return decisionCommit
	case "pending":
		return decisionRetry
	default:
		return decisionCommit
	}
}

func writePaymentSucceeded(ctx context.Context, w *kafka.Writer, orderID uuid.UUID, paymentID string) error {
	inner, err := json.Marshal(events.PaymentResultPayload{
		OrderID:   orderID.String(),
		PaymentID: paymentID,
	})
	if err != nil {
		return err
	}
	out, err := json.Marshal(events.Envelope{
		Type:    "PaymentSucceeded",
		Payload: inner,
	})
	if err != nil {
		return err
	}
	msg := kafka.Message{Key: []byte(orderID.String()), Value: out}
	return w.WriteMessages(ctx, msg)
}

type dlqRecord struct {
	Reason      string `json:"reason"`
	Detail      string `json:"detail,omitempty"`
	SourceTopic string `json:"source_topic"`
	Partition   int    `json:"partition"`
	Offset      int64  `json:"offset"`
	PayloadB64  string `json:"payload_b64"`
}

func writeDLQ(ctx context.Context, bus *Bus, m kafka.Message, reason, detail string) error {
	if bus.DLQ == nil || bus.TopicSrc == "" {
		return errors.New("dlq writer not configured")
	}
	payload, err := json.Marshal(dlqRecord{
		Reason:      reason,
		Detail:      detail,
		SourceTopic: bus.TopicSrc,
		Partition:   m.Partition,
		Offset:      m.Offset,
		PayloadB64:  base64.StdEncoding.EncodeToString(m.Value),
	})
	if err != nil {
		return err
	}
	return bus.DLQ.WriteMessages(ctx, kafka.Message{
		Key:   m.Key,
		Value: payload,
	})
}
