package domain

import (
	"time"

	"github.com/google/uuid"
)

// Client represents a bank client. Only the minimal fields the challenge
// asks for. In a real system there would also be ID document, address, etc.
type Client struct {
	ID        uuid.UUID
	Name      string
	Email     string
	CreatedAt time.Time
}

// NewClient builds a Client with a generated ID and current timestamp.
// Syntactic validation of Name/Email lives in the HTTP handler (via tags),
// here we just assign the values.
func NewClient(name, email string) Client {
	return Client{
		ID:        uuid.New(),
		Name:      name,
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}
}
