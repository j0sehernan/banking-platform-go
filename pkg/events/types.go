package events

// Topics y nombres de eventos que viajan por el bus.
// Los mantengo en un solo lugar para evitar typos sueltos en cada servicio.
const (
	// Topics
	TopicAccountsEvents       = "accounts.events"
	TopicTransactionsCommands = "transactions.commands"
	TopicAccountsTxResults    = "accounts.tx-results"
	TopicTransactionsEvents   = "transactions.events"
	TopicDLQ                  = "dlq"

	// Eventos de accounts
	EventClientCreated  = "ClientCreated"
	EventAccountCreated = "AccountCreated"
	EventBalanceUpdated = "BalanceUpdated"

	// Comandos / eventos de transactions
	EventTransactionRequested = "TransactionRequested"
	EventTransactionCompleted = "TransactionCompleted"
	EventTransactionRejected  = "TransactionRejected"

	// Resultados que accounts manda de vuelta a transactions
	EventAccountsTransferApplied = "AccountsTransferApplied"
	EventAccountsTransferFailed  = "AccountsTransferFailed"
)

// Tipos de transacción soportados.
const (
	TxTypeDeposit  = "DEPOSIT"
	TxTypeWithdraw = "WITHDRAW"
	TxTypeTransfer = "TRANSFER"
)

// Estados de la transacción (máquina de estados).
const (
	TxStatusPending   = "PENDING"
	TxStatusCompleted = "COMPLETED"
	TxStatusRejected  = "REJECTED"
)

// ===== Payloads =====

// ClientCreatedPayload — accounts publica esto al crear un cliente.
type ClientCreatedPayload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

// AccountCreatedPayload — accounts publica esto al crear una cuenta.
type AccountCreatedPayload struct {
	ID        string `json:"id"`
	ClientID  string `json:"client_id"`
	Currency  string `json:"currency"`
	Balance   string `json:"balance"` // como string para no perder precisión
	CreatedAt string `json:"created_at"`
}

// BalanceUpdatedPayload — accounts publica esto al cambiar el saldo.
type BalanceUpdatedPayload struct {
	AccountID  string `json:"account_id"`
	OldBalance string `json:"old_balance"`
	NewBalance string `json:"new_balance"`
	Reason     string `json:"reason"` // ej: "deposit", "withdraw", "transfer-out"
	UpdatedAt  string `json:"updated_at"`
}

// TransactionRequestedPayload — transactions publica esto en
// transactions.commands para que accounts ejecute la operación.
type TransactionRequestedPayload struct {
	TransactionID string `json:"transaction_id"`
	Type          string `json:"type"` // DEPOSIT | WITHDRAW | TRANSFER
	FromAccountID string `json:"from_account_id,omitempty"`
	ToAccountID   string `json:"to_account_id,omitempty"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
}

// AccountsTransferAppliedPayload — accounts contesta a transactions
// que la operación fue aplicada exitosamente.
type AccountsTransferAppliedPayload struct {
	TransactionID string `json:"transaction_id"`
	AppliedAt     string `json:"applied_at"`
}

// AccountsTransferFailedPayload — accounts contesta a transactions
// que la operación falló y por qué.
type AccountsTransferFailedPayload struct {
	TransactionID string `json:"transaction_id"`
	Reason        string `json:"reason"` // insufficient_funds | account_not_found | currency_mismatch
	Message       string `json:"message"`
	FailedAt      string `json:"failed_at"`
}

// TransactionCompletedPayload — transactions anuncia el resultado final OK.
type TransactionCompletedPayload struct {
	TransactionID string `json:"transaction_id"`
	Type          string `json:"type"`
	FromAccountID string `json:"from_account_id,omitempty"`
	ToAccountID   string `json:"to_account_id,omitempty"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	CompletedAt   string `json:"completed_at"`
}

// TransactionRejectedPayload — transactions anuncia el resultado final KO.
type TransactionRejectedPayload struct {
	TransactionID string `json:"transaction_id"`
	Type          string `json:"type"`
	FromAccountID string `json:"from_account_id,omitempty"`
	ToAccountID   string `json:"to_account_id,omitempty"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	Reason        string `json:"reason"`
	Message       string `json:"message"`
	RejectedAt    string `json:"rejected_at"`
}
