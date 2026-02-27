// Package port defines the interfaces (ports) for external dependencies.
// Following hexagonal architecture, these ports decouple the domain/service
// layer from concrete implementations.
package port

import (
	"context"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// ProfileFetcher retrieves customer profile data.
type ProfileFetcher interface {
	GetProfile(ctx context.Context, customerID string) (*domain.CustomerProfile, error)
}

// TransactionsFetcher retrieves customer transaction data.
type TransactionsFetcher interface {
	GetTransactions(ctx context.Context, customerID string) ([]domain.Transaction, error)
}

// AgentCaller invokes the AI Agent service.
type AgentCaller interface {
	Call(ctx context.Context, req *domain.AgentRequest) (*domain.AgentResponse, error)
}

// Cache provides generic caching with TTL.
type Cache[T any] interface {
	Get(key string) (T, bool)
	Set(key string, value T)
	Delete(key string)
}

// BankingStore defines all data operations for banking features.
// Implemented by the Supabase adapter (or any other persistence layer).
type BankingStore interface {
	// Accounts
	ListAccounts(ctx context.Context, customerID string) ([]domain.Account, error)
	GetAccount(ctx context.Context, customerID, accountID string) (*domain.Account, error)
	GetPrimaryAccount(ctx context.Context, customerID string) (*domain.Account, error)

	// PIX Keys
	ListPixKeys(ctx context.Context, customerID string) ([]domain.PixKey, error)
	LookupPixKey(ctx context.Context, keyType, keyValue string) (*domain.PixKey, error)

	// PIX Transfers
	CreatePixTransfer(ctx context.Context, customerID string, req *domain.PixTransferRequest) (*domain.PixTransfer, error)
	ListPixTransfers(ctx context.Context, customerID string, page, pageSize int) ([]domain.PixTransfer, error)
	GetPixTransfer(ctx context.Context, customerID, transferID string) (*domain.PixTransfer, error)
	UpdatePixTransferStatus(ctx context.Context, transferID, status string) error

	// Scheduled Transfers
	CreateScheduledTransfer(ctx context.Context, customerID string, req *domain.ScheduledTransferRequest) (*domain.ScheduledTransfer, error)
	ListScheduledTransfers(ctx context.Context, customerID string) ([]domain.ScheduledTransfer, error)
	GetScheduledTransfer(ctx context.Context, customerID, transferID string) (*domain.ScheduledTransfer, error)
	UpdateScheduledTransferStatus(ctx context.Context, transferID, status string) error

	// Credit Cards
	CreateCreditCard(ctx context.Context, customerID string, req *domain.CreditCardRequest) (*domain.CreditCard, error)
	ListCreditCards(ctx context.Context, customerID string) ([]domain.CreditCard, error)
	GetCreditCard(ctx context.Context, customerID, cardID string) (*domain.CreditCard, error)
	UpdateCreditCardStatus(ctx context.Context, cardID, status string) error

	// Credit Card Transactions
	ListCreditCardTransactions(ctx context.Context, customerID, cardID string, page, pageSize int) ([]domain.CreditCardTransaction, error)

	// Credit Card Invoices
	ListCreditCardInvoices(ctx context.Context, customerID, cardID string) ([]domain.CreditCardInvoice, error)
	GetCreditCardInvoice(ctx context.Context, customerID, cardID, invoiceID string) (*domain.CreditCardInvoice, error)
	GetCreditCardInvoiceByMonth(ctx context.Context, customerID, cardID, month string) (*domain.CreditCardInvoice, error)

	// Bill Payments
	CreateBillPayment(ctx context.Context, customerID string, req *domain.BillPaymentRequest, validation *domain.BarcodeValidationResponse) (*domain.BillPayment, error)
	ListBillPayments(ctx context.Context, customerID string, page, pageSize int) ([]domain.BillPayment, error)
	GetBillPayment(ctx context.Context, customerID, billID string) (*domain.BillPayment, error)
	UpdateBillPaymentStatus(ctx context.Context, billID, status string) error

	// Debit Purchases
	ListDebitPurchases(ctx context.Context, customerID string, page, pageSize int) ([]domain.DebitPurchase, error)
	CreateDebitPurchase(ctx context.Context, customerID string, req *domain.DebitPurchaseRequest) (*domain.DebitPurchase, error)

	// Spending Analytics
	GetSpendingSummary(ctx context.Context, customerID, periodType string) (*domain.SpendingSummary, error)
	ListBudgets(ctx context.Context, customerID string) ([]domain.SpendingBudget, error)
	CreateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error)
	UpdateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error)

	// Favorites
	ListFavorites(ctx context.Context, customerID string) ([]domain.Favorite, error)
	CreateFavorite(ctx context.Context, fav *domain.Favorite) (*domain.Favorite, error)
	DeleteFavorite(ctx context.Context, customerID, favoriteID string) error

	// Transaction Limits
	ListTransactionLimits(ctx context.Context, customerID string) ([]domain.TransactionLimit, error)
	GetTransactionLimit(ctx context.Context, customerID, txType string) (*domain.TransactionLimit, error)
	UpdateTransactionLimit(ctx context.Context, limit *domain.TransactionLimit) (*domain.TransactionLimit, error)

	// Notifications
	ListNotifications(ctx context.Context, customerID string, unreadOnly bool, page, pageSize int) ([]domain.Notification, error)
	MarkNotificationRead(ctx context.Context, notifID string) error

	// Transactions (bank statement — needed for summary)
	GetTransactionSummary(ctx context.Context, customerID string) (*domain.TransactionSummary, error)
}

// AuthStore defines all data operations for the authentication system.
type AuthStore interface {
	// Customer lookup
	GetCustomerByID(ctx context.Context, customerID string) (*domain.CustomerProfile, error)
	GetCustomerByDocument(ctx context.Context, document string) (*domain.CustomerProfile, error)
	GetCustomerByBankDetails(ctx context.Context, document, agencia, conta string) (*domain.CustomerProfile, error)
	GetCustomerByCPF(ctx context.Context, cpf string) (*domain.CustomerProfile, error)

	// Registration
	CreateCustomerWithAccount(ctx context.Context, req *domain.RegisterRequest, passwordHash string) (*domain.RegisterResponse, error)

	// Credentials
	GetCredentials(ctx context.Context, customerID string) (*domain.AuthCredential, error)
	UpdateCredentials(ctx context.Context, customerID string, updates map[string]any) error

	// Refresh tokens
	StoreRefreshToken(ctx context.Context, customerID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*domain.AuthRefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllRefreshTokens(ctx context.Context, customerID string) error

	// Password reset codes
	StoreResetCode(ctx context.Context, customerID, code string, expiresAt time.Time) error
	GetValidResetCode(ctx context.Context, customerID, code string) (*domain.AuthPasswordResetCode, error)
	MarkResetCodeUsed(ctx context.Context, codeID string) error

	// Profile updates
	UpdateCustomerProfile(ctx context.Context, customerID string, updates map[string]any) (*domain.CustomerProfile, error)
	UpdateRepresentative(ctx context.Context, customerID string, updates map[string]any) (*domain.CustomerProfile, error)
}
