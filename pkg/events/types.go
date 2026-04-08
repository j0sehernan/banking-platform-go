package events

// Topics and event names traveling through the bus.
// Keeping them in a single place avoids stray typos in each service.
const (
	// Topics
	TopicAccountsEvents       = "accounts.events"
	TopicTransactionsCommands = "transactions.commands"
	TopicAccountsTxResults    = "accounts.tx-results"
	TopicTransactionsEvents   = "transactions.events"
	TopicDLQ                  = "dlq"

	// Accounts events
	EventClientCreated  = "ClientCreated"
	EventAccountCreated = "AccountCreated"
	EventBalanceUpdated = "BalanceUpdated"

	// Transactions commands / events
	EventTransactionRequested = "TransactionRequested"
	EventTransactionCompleted = "TransactionCompleted"
	EventTransactionRejected  = "TransactionRejected"

	// Results sent from accounts back to transactions
	EventAccountsTransferApplied = "AccountsTransferApplied"
	EventAccountsTransferFailed  = "AccountsTransferFailed"
)

// Supported transaction types.
const (
	TxTypeDeposit  = "DEPOSIT"
	TxTypeWithdraw = "WITHDRAW"
	TxTypeTransfer = "TRANSFER"
)

// Transaction states (state machine).
const (
	TxStatusPending   = "PENDING"
	TxStatusCompleted = "COMPLETED"
	TxStatusRejected  = "REJECTED"
)

// ===== Payloads =====

// ClientCreatedPayload — accounts publishes this when a client is created.
type ClientCreatedPayload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

// AccountCreatedPayload — accounts publishes this when an account is created.
type AccountCreatedPayload struct {
	ID        string `json:"id"`
	ClientID  string `json:"client_id"`
	Currency  string `json:"currency"`
	Balance   string `json:"balance"` // string to preserve decimal precision
	CreatedAt string `json:"created_at"`
}

// BalanceUpdatedPayload — accounts publishes this when a balance changes.
type BalanceUpdatedPayload struct {
	AccountID  string `json:"account_id"`
	OldBalance string `json:"old_balance"`
	NewBalance string `json:"new_balance"`
	Reason     string `json:"reason"` // e.g. "deposit", "withdraw", "transfer-out"
	UpdatedAt  string `json:"updated_at"`
}

// TransactionRequestedPayload — transactions publishes this on
// transactions.commands so accounts executes the operation.
type TransactionRequestedPayload struct {
	TransactionID string `json:"transaction_id"`
	Type          string `json:"type"` // DEPOSIT | WITHDRAW | TRANSFER
	FromAccountID string `json:"from_account_id,omitempty"`
	ToAccountID   string `json:"to_account_id,omitempty"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
}

// AccountsTransferAppliedPayload — accounts replies to transactions
// that the operation was applied successfully.
type AccountsTransferAppliedPayload struct {
	TransactionID string `json:"transaction_id"`
	AppliedAt     string `json:"applied_at"`
}

// AccountsTransferFailedPayload — accounts replies to transactions
// that the operation failed and why.
type AccountsTransferFailedPayload struct {
	TransactionID string `json:"transaction_id"`
	Reason        string `json:"reason"` // insufficient_funds | account_not_found | currency_mismatch
	Message       string `json:"message"`
	FailedAt      string `json:"failed_at"`
}

// TransactionCompletedPayload — transactions announces the final OK result.
type TransactionCompletedPayload struct {
	TransactionID string `json:"transaction_id"`
	Type          string `json:"type"`
	FromAccountID string `json:"from_account_id,omitempty"`
	ToAccountID   string `json:"to_account_id,omitempty"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	CompletedAt   string `json:"completed_at"`
}

// TransactionRejectedPayload — transactions announces the final KO result.
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
