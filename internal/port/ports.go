// Package port defines the interfaces (ports) for external dependencies.
// Following hexagonal architecture, these ports decouple the domain/service
// layer from concrete implementations.
//
// Individual store interfaces are defined in separate files:
//   - account_port.go  → AccountStore
//   - pix_port.go      → PixKeyStore, PixTransferStore, PixReceiptStore,
//                         CustomerLookupStore, ScheduledTransferStore
//   - cards_port.go    → CreditCardStore, CreditCardTransactionStore,
//                         CreditCardInvoiceStore
//   - billing_port.go  → BillingStore
//   - analytics_port.go→ AnalyticsStore
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

// BankingStore composes all domain-specific store interfaces into a single
// aggregate interface. This is consumed by BankingService which orchestrates
// cross-domain operations. The Supabase Client satisfies all sub-interfaces.
type BankingStore interface {
	AccountStore
	PixKeyStore
	PixTransferStore
	PixReceiptStore
	CustomerLookupStore
	ScheduledTransferStore
	CreditCardStore
	CreditCardTransactionStore
	CreditCardInvoiceStore
	BillingStore
	AnalyticsStore
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

	// Dev auth (DEV_AUTH=true only) — plain-text password lookup in dev_logins table
	DevLoginLookup(ctx context.Context, cpf, password string) (*domain.CustomerProfile, error)
}
