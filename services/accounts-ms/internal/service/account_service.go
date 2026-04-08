// Package service contiene los casos de uso del microservicio.
// Orquesta domain + repo, sin tocar HTTP ni Kafka directamente.
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/j0sehernan/banking-platform-go/pkg/events"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AccountService maneja clientes y cuentas. Tiene acceso al pool de
// Postgres porque algunas operaciones requieren transacciones que
// abarcan varios repos (crear cuenta + insertar outbox).
type AccountService struct {
	pool *pgxpool.Pool
}

func NewAccountService(pool *pgxpool.Pool) *AccountService {
	return &AccountService{pool: pool}
}

// CreateClient crea un cliente y publica ClientCreated vía outbox.
// Todo en una sola transacción → atomicidad garantizada.
func (s *AccountService) CreateClient(ctx context.Context, name, email string) (*domain.Client, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // si commit ok, este rollback es no-op

	clientRepo := newClientRepoTx(tx)
	outboxRepo := newOutboxRepoTx(tx)

	client := domain.NewClient(name, email)
	if err := clientRepo.Create(ctx, client); err != nil {
		return nil, err
	}

	// publicar evento via outbox (en la misma tx)
	payload := events.ClientCreatedPayload{
		ID:        client.ID.String(),
		Name:      client.Name,
		Email:     client.Email,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}
	env, err := events.NewEnvelope(events.EventClientCreated, payload, "")
	if err != nil {
		return nil, err
	}
	envBytes, _ := json.Marshal(env)
	if err := outboxRepo.Insert(ctx, events.TopicAccountsEvents, client.ID.String(), envBytes, nil); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &client, nil
}

// CreateAccount crea una cuenta para un cliente existente y publica AccountCreated.
func (s *AccountService) CreateAccount(ctx context.Context, clientID uuid.UUID, currency string) (*domain.Account, error) {
	// validamos primero que el cliente existe
	clientRepo := newClientRepoPool(s.pool)
	if _, err := clientRepo.GetByID(ctx, clientID.String()); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	accountRepo := newAccountRepoTx(tx)
	outboxRepo := newOutboxRepoTx(tx)

	acc, err := domain.NewAccount(clientID, currency)
	if err != nil {
		return nil, err
	}
	if err := accountRepo.Create(ctx, acc); err != nil {
		return nil, err
	}

	payload := events.AccountCreatedPayload{
		ID:        acc.ID.String(),
		ClientID:  acc.ClientID.String(),
		Currency:  acc.Currency,
		Balance:   acc.Balance.String(),
		CreatedAt: acc.CreatedAt.Format(time.RFC3339),
	}
	env, err := events.NewEnvelope(events.EventAccountCreated, payload, "")
	if err != nil {
		return nil, err
	}
	envBytes, _ := json.Marshal(env)
	if err := outboxRepo.Insert(ctx, events.TopicAccountsEvents, acc.ID.String(), envBytes, nil); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetAccount lee una cuenta por id.
func (s *AccountService) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	repo := newAccountRepoPool(s.pool)
	return repo.GetByID(ctx, id)
}

// ListAccounts devuelve todas las cuentas (paginado simple, ver repo).
func (s *AccountService) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	repo := newAccountRepoPool(s.pool)
	return repo.ListAll(ctx)
}

// GetClient lee un cliente por id.
func (s *AccountService) GetClient(ctx context.Context, id string) (*domain.Client, error) {
	repo := newClientRepoPool(s.pool)
	return repo.GetByID(ctx, id)
}

// ListAccountsByClient devuelve las cuentas de un cliente.
func (s *AccountService) ListAccountsByClient(ctx context.Context, clientID uuid.UUID) ([]domain.Account, error) {
	repo := newAccountRepoPool(s.pool)
	return repo.ListByClient(ctx, clientID)
}
