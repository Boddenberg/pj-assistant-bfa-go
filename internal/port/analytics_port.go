package port

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// AnalyticsStore handles spending analytics, budgets, favorites,
// limits, notifications, and transaction history operations.
type AnalyticsStore interface {
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

	// Transaction History
	GetTransactionSummary(ctx context.Context, customerID string) (*domain.TransactionSummary, error)
	ListTransactions(ctx context.Context, customerID string, from, to string) ([]domain.Transaction, error)
	InsertTransaction(ctx context.Context, data map[string]any) error
}
