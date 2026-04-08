package domain

import (
	"time"

	"github.com/google/uuid"
)

// Client es un cliente del banco. Mantengo solo los campos mínimos
// que pide el enunciado. En la realidad acá habría DNI, dirección, etc.
type Client struct {
	ID        uuid.UUID
	Name      string
	Email     string
	CreatedAt time.Time
}

// NewClient construye un Client con ID generado y timestamp actual.
// La validación sintáctica de Name/Email vive en el handler HTTP (tags),
// acá solo asignamos los valores.
func NewClient(name, email string) Client {
	return Client{
		ID:        uuid.New(),
		Name:      name,
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}
}
