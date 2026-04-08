// Package events define el envelope estándar que viaja en Kafka y los tipos
// de eventos que cada microservicio puede publicar o consumir.
package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope es el formato común de todos los mensajes que viajan por Kafka.
// Tener un envelope estándar simplifica idempotencia, tracing y deserialización.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Version       int             `json:"version"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// NewEnvelope crea un envelope con un event_id nuevo y timestamp actual.
// El payload se serializa acá para evitar errores en el call site.
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

// Decode deserializa el payload del envelope al tipo destino.
func (e Envelope) Decode(dst any) error {
	return json.Unmarshal(e.Payload, dst)
}

// MarshalBinary serializa el envelope para enviarlo a Kafka.
func (e Envelope) MarshalBinary() ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalEnvelope parsea un envelope desde bytes (lo usa el consumer).
func UnmarshalEnvelope(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return Envelope{}, err
	}
	return e, nil
}
