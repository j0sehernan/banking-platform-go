package events

import (
	"testing"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	payload := TransactionRequestedPayload{
		TransactionID: "tx_123",
		Type:          TxTypeTransfer,
		FromAccountID: "acc_a",
		ToAccountID:   "acc_b",
		Amount:        "300.00",
		Currency:      "USD",
	}

	env, err := NewEnvelope(EventTransactionRequested, payload, "corr_1")
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.EventID == "" {
		t.Error("expected event_id to be set")
	}
	if env.EventType != EventTransactionRequested {
		t.Errorf("expected event_type %q, got %q", EventTransactionRequested, env.EventType)
	}

	// serialize and deserialize
	bytes, err := env.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	parsed, err := UnmarshalEnvelope(bytes)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope failed: %v", err)
	}

	if parsed.EventID != env.EventID {
		t.Errorf("event_id mismatch: %q vs %q", parsed.EventID, env.EventID)
	}

	var decoded TransactionRequestedPayload
	if err := parsed.Decode(&decoded); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}

	if decoded.TransactionID != payload.TransactionID {
		t.Errorf("payload mismatch: %q vs %q", decoded.TransactionID, payload.TransactionID)
	}
	if decoded.Amount != "300.00" {
		t.Errorf("expected amount '300.00', got %q", decoded.Amount)
	}
}
