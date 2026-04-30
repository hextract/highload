package events

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
}

func NewPaymentResultsDeadLetterWriter(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        TopicPaymentResultsDLQ,
		Balancer:     &kafka.RoundRobin{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		WriteTimeout: 15 * time.Second,
	}
}

type PaymentResultHandler interface {
	OnPaymentSucceeded(ctx context.Context, orderID uuid.UUID) error
	OnPaymentFailed(ctx context.Context, orderID uuid.UUID) error
}

func RunPaymentResultConsumer(ctx context.Context, brokers []string, h PaymentResultHandler, dlq *kafka.Writer) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:               brokers,
		GroupID:               "order-service-payments",
		Topic:                 TopicPaymentResults,
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
		switch consumePaymentResult(ctx, h, dlq, &m) {
		case consumeCommit:
			if err := r.CommitMessages(ctx, m); err != nil {
				slog.Error("commit", "err", err)
			}
		case consumeBackoff:
			time.Sleep(400 * time.Millisecond)
		}
	}
}

type consumeOutcome int

const (
	consumeCommit consumeOutcome = iota
	consumeBackoff
)

func consumePaymentResult(ctx context.Context, h PaymentResultHandler, dlq *kafka.Writer, m *kafka.Message) consumeOutcome {
	var env Envelope
	if err := json.Unmarshal(m.Value, &env); err != nil {
		if writeResultsDLQ(ctx, dlq, m, TopicPaymentResults, "json_envelope", err.Error()) != nil {
			return consumeBackoff
		}
		return consumeCommit
	}
	switch env.Type {
	case "PaymentSucceeded":
		var p PaymentResultPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			if writeResultsDLQ(ctx, dlq, m, TopicPaymentResults, "json_payload_succeeded", err.Error()) != nil {
				return consumeBackoff
			}
			return consumeCommit
		}
		oid, err := uuid.Parse(p.OrderID)
		if err != nil {
			if writeResultsDLQ(ctx, dlq, m, TopicPaymentResults, "bad_order_id", err.Error()) != nil {
				return consumeBackoff
			}
			return consumeCommit
		}
		if err := h.OnPaymentSucceeded(ctx, oid); err != nil {
			slog.Error("on succeeded", "err", err)
			return consumeBackoff
		}
		return consumeCommit
	case "PaymentFailed":
		var p PaymentResultPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			if writeResultsDLQ(ctx, dlq, m, TopicPaymentResults, "json_payload_failed", err.Error()) != nil {
				return consumeBackoff
			}
			return consumeCommit
		}
		oid, err := uuid.Parse(p.OrderID)
		if err != nil {
			if writeResultsDLQ(ctx, dlq, m, TopicPaymentResults, "bad_order_id", err.Error()) != nil {
				return consumeBackoff
			}
			return consumeCommit
		}
		if err := h.OnPaymentFailed(ctx, oid); err != nil {
			slog.Error("on failed", "err", err)
			return consumeBackoff
		}
		return consumeCommit
	default:
		if writeResultsDLQ(ctx, dlq, m, TopicPaymentResults, "unexpected_type", env.Type) != nil {
			return consumeBackoff
		}
		return consumeCommit
	}
}

type resultsDLQ struct {
	Reason      string `json:"reason"`
	Detail      string `json:"detail,omitempty"`
	SourceTopic string `json:"source_topic"`
	Partition   int    `json:"partition"`
	Offset      int64  `json:"offset"`
	PayloadB64  string `json:"payload_b64"`
}

func writeResultsDLQ(ctx context.Context, dlq *kafka.Writer, m *kafka.Message, topic, reason, detail string) error {
	if dlq == nil {
		return errors.New("dlq writer not configured")
	}
	raw, err := json.Marshal(resultsDLQ{
		Reason:      reason,
		Detail:      detail,
		SourceTopic: topic,
		Partition:   m.Partition,
		Offset:      m.Offset,
		PayloadB64:  base64.StdEncoding.EncodeToString(m.Value),
	})
	if err != nil {
		return err
	}
	return dlq.WriteMessages(ctx, kafka.Message{Key: m.Key, Value: raw})
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
