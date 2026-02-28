// Package service provides the business logic layer (use cases).
// BankingService handles all banking operations: PIX, transfers,
// credit cards, bill payments, analytics, etc.
package service

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

var bankTracer = otel.Tracer("service/banking")

// BankingService orchestrates all banking operations via the Supabase store.
type BankingService struct {
	store   port.BankingStore
	metrics *observability.Metrics
	logger  *zap.Logger
}

// NewBankingService creates a new banking service.
func NewBankingService(store port.BankingStore, metrics *observability.Metrics, logger *zap.Logger) *BankingService {
	return &BankingService{store: store, metrics: metrics, logger: logger}
}

// ============================================================
// Accounts
// ============================================================

func (s *BankingService) ListAccounts(ctx context.Context, customerID string) ([]domain.Account, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListAccounts")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID))

	return s.store.ListAccounts(ctx, customerID)
}

func (s *BankingService) GetAccount(ctx context.Context, customerID, accountID string) (*domain.Account, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetAccount")
	defer span.End()

	return s.store.GetAccount(ctx, customerID, accountID)
}

// GetPrimaryAccount returns the customer's primary (first active) account.
func (s *BankingService) GetPrimaryAccount(ctx context.Context, customerID string) (*domain.Account, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetPrimaryAccount")
	defer span.End()

	return s.store.GetPrimaryAccount(ctx, customerID)
}
