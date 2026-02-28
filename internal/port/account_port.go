package port

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// AccountStore handles account data operations.
type AccountStore interface {
	ListAccounts(ctx context.Context, customerID string) ([]domain.Account, error)
	GetAccount(ctx context.Context, customerID, accountID string) (*domain.Account, error)
	GetPrimaryAccount(ctx context.Context, customerID string) (*domain.Account, error)
	UpdateAccountBalance(ctx context.Context, customerID string, delta float64) (*domain.Account, error)
}
