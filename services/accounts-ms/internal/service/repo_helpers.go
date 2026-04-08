package service

import (
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/repo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tiny helpers to construct repos over pool or over tx without
// duplicating boilerplate. The DBTX interface in repo accepts both.

func newClientRepoPool(p *pgxpool.Pool) *repo.ClientRepo   { return repo.NewClientRepo(p) }
func newAccountRepoPool(p *pgxpool.Pool) *repo.AccountRepo { return repo.NewAccountRepo(p) }

func newClientRepoTx(tx pgx.Tx) *repo.ClientRepo   { return repo.NewClientRepo(tx) }
func newAccountRepoTx(tx pgx.Tx) *repo.AccountRepo { return repo.NewAccountRepo(tx) }
func newOutboxRepoTx(tx pgx.Tx) *repo.OutboxRepo   { return repo.NewOutboxRepo(tx) }
