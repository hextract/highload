package events

import "encoding/json"

const (
	TopicPaymentRequests    = "payment.requests"
	TopicPaymentResults     = "payment.results"
	TopicPaymentResultsDLQ = "payment.results.dlq"
)

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type PaymentRequestPayload struct {
	OrderID        string `json:"order_id"`
	PaymentID      string `json:"payment_id"`
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	PaymentMethod  string `json:"payment_method"`
}

type PaymentResultPayload struct {
	OrderID   string `json:"order_id"`
	PaymentID string `json:"payment_id"`
	Error     string `json:"error,omitempty"`
}
