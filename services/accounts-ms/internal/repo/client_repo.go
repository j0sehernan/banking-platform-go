// Package repo implementa el acceso a Postgres con pgx (sin ORM).
// Las queries son simples y SQL crudo, lo cual es idiomático en Go.
// Todas usan placeholders ($1, $2...) para evitar SQL injection.
package repo

import (
	"context"
	"errors"

	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgUniqueViolation es el SQLSTATE que Postgres devuelve cuando se viola
// un UNIQUE constraint. Lo usamos para detectar email duplicado.
const pgUniqueViolation = "23505"

type ClientRepo struct {
	db DBTX
}

func NewClientRepo(db DBTX) *ClientRepo {
	return &ClientRepo{db: db}
}

// Create inserta un cliente. Si el email ya existe, devuelve ErrEmailAlreadyExists.
func (r *ClientRepo) Create(ctx context.Context, c domain.Client) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO clients (id, name, email, created_at) VALUES ($1, $2, $3, $4)`,
		c.ID, c.Name, c.Email, c.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.ErrEmailAlreadyExists
		}
		return err
	}
	return nil
}

// GetByID busca un cliente. Devuelve ErrClientNotFound si no existe.
func (r *ClientRepo) GetByID(ctx context.Context, id string) (*domain.Client, error) {
	var c domain.Client
	err := r.db.QueryRow(ctx,
		`SELECT id, name, email, created_at FROM clients WHERE id = $1`,
		id,
	).Scan(&c.ID, &c.Name, &c.Email, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrClientNotFound
		}
		return nil, err
	}
	return &c, nil
}
