// Package events defines the standard envelope that travels through Kafka
// and the event types each microservice can publish or consume.
package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope is the common format for all messages sent through Kafka.
// Having a standard envelope simplifies idempotency, tracing, and decoding.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Version       int             `json:"version"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// NewEnvelope builds an envelope with a fresh event_id and current timestamp.
// Payload is serialized here to avoid errors at the call site.
func NewEnvelope(eventType string, payload any, correlationID string) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		CorrelationID: correlationID,
		Payload:       raw,
	}, nil
}

// Decode unmarshals the envelope payload into the destination type.
func (e Envelope) Decode(dst any) error {
	return json.Unmarshal(e.Payload, dst)
}

// MarshalBinary serializes the envelope for sending to Kafka.
func (e Envelope) MarshalBinary() ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalEnvelope parses an envelope from raw bytes (used by the consumer).
func UnmarshalEnvelope(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return Envelope{}, err
	}
	return e, nil
}
