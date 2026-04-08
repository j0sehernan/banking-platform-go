// Package service contains the use cases of the microservice.
// It orchestrates domain + repo, without touching HTTP or Kafka directly.
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

// AccountService manages clients and accounts. It has access to the
// Postgres pool because some operations need transactions that span
// multiple repos (create account + insert outbox).
type AccountService struct {
	pool *pgxpool.Pool
}

func NewAccountService(pool *pgxpool.Pool) *AccountService {
	return &AccountService{pool: pool}
}

// CreateClient creates a client and publishes ClientCreated via outbox.
// Everything in a single transaction → atomicity guaranteed.
func (s *AccountService) CreateClient(ctx context.Context, name, email string) (*domain.Client, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // if commit ok, this rollback is a no-op

	clientRepo := newClientRepoTx(tx)
	outboxRepo := newOutboxRepoTx(tx)

	client := domain.NewClient(name, email)
	if err := clientRepo.Create(ctx, client); err != nil {
		return nil, err
	}

	// publish event via outbox (in the same tx)
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

// CreateAccount creates an account for an existing client and publishes AccountCreated.
func (s *AccountService) CreateAccount(ctx context.Context, clientID uuid.UUID, currency string) (*domain.Account, error) {
	// validate first that the client exists
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

// GetAccount reads an account by id.
func (s *AccountService) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	repo := newAccountRepoPool(s.pool)
	return repo.GetByID(ctx, id)
}

// ListAccounts returns all accounts (simple pagination, see repo).
func (s *AccountService) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	repo := newAccountRepoPool(s.pool)
	return repo.ListAll(ctx)
}

// GetClient reads a client by id.
func (s *AccountService) GetClient(ctx context.Context, id string) (*domain.Client, error) {
	repo := newClientRepoPool(s.pool)
	return repo.GetByID(ctx, id)
}

// ListAccountsByClient returns the accounts of a client.
func (s *AccountService) ListAccountsByClient(ctx context.Context, clientID uuid.UUID) ([]domain.Account, error) {
	repo := newAccountRepoPool(s.pool)
	return repo.ListByClient(ctx, clientID)
}
