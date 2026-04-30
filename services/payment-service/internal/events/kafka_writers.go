package events

import (
	"time"

	"github.com/segmentio/kafka-go"
)

func NewPaymentResultWriter(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        TopicPaymentResults,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
}

func NewRequestsDeadLetterWriter(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        TopicPaymentRequestsDLQ,
		Balancer:     &kafka.RoundRobin{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		WriteTimeout: 15 * time.Second,
	}
}
