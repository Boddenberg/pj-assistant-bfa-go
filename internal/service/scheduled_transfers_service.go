package service

import (
	"context"
	"fmt"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"go.uber.org/zap"
)

// ============================================================
// Scheduled Transfers
// ============================================================

func (s *BankingService) CreateScheduledTransfer(ctx context.Context, customerID string, req *domain.ScheduledTransferRequest) (*domain.ScheduledTransfer, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreateScheduledTransfer")
	defer span.End()

	if req.Amount <= 0 {
		return nil, &domain.ErrValidation{Field: "amount", Message: "must be positive"}
	}
	if req.ScheduledDate == "" {
		return nil, &domain.ErrValidation{Field: "scheduled_date", Message: "required"}
	}
	if req.IdempotencyKey == "" {
		return nil, &domain.ErrValidation{Field: "idempotency_key", Message: "required"}
	}

	// Validate date is in the future
	schedDate, err := time.Parse("2006-01-02", req.ScheduledDate)
	if err != nil {
		return nil, &domain.ErrValidation{Field: "scheduled_date", Message: "invalid format, use YYYY-MM-DD"}
	}
	if schedDate.Before(time.Now().Truncate(24 * time.Hour)) {
		return nil, &domain.ErrValidation{Field: "scheduled_date", Message: "must be today or in the future"}
	}

	// Check account
	_, err = s.store.GetAccount(ctx, customerID, req.SourceAccountID)
	if err != nil {
		return nil, err
	}

	transfer, err := s.store.CreateScheduledTransfer(ctx, customerID, req)
	if err != nil {
		s.logger.Error("failed to create scheduled transfer", zap.String("customer_id", customerID), zap.Error(err))
		return nil, err
	}

	s.logger.Info("scheduled transfer created",
		zap.String("customer_id", customerID),
		zap.String("transfer_id", transfer.ID),
		zap.Float64("amount", req.Amount),
		zap.String("scheduled_date", req.ScheduledDate),
	)

	return transfer, nil
}

func (s *BankingService) ListScheduledTransfers(ctx context.Context, customerID string) ([]domain.ScheduledTransfer, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListScheduledTransfers")
	defer span.End()

	return s.store.ListScheduledTransfers(ctx, customerID)
}

func (s *BankingService) GetScheduledTransfer(ctx context.Context, customerID, transferID string) (*domain.ScheduledTransfer, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetScheduledTransfer")
	defer span.End()

	return s.store.GetScheduledTransfer(ctx, customerID, transferID)
}

func (s *BankingService) CancelScheduledTransfer(ctx context.Context, customerID, transferID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.CancelScheduledTransfer")
	defer span.End()

	transfer, err := s.store.GetScheduledTransfer(ctx, customerID, transferID)
	if err != nil {
		return err
	}
	if transfer.Status != "scheduled" && transfer.Status != "paused" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot cancel transfer with status '%s'", transfer.Status)}
	}

	return s.store.UpdateScheduledTransferStatus(ctx, transferID, "cancelled")
}

func (s *BankingService) PauseScheduledTransfer(ctx context.Context, customerID, transferID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.PauseScheduledTransfer")
	defer span.End()

	transfer, err := s.store.GetScheduledTransfer(ctx, customerID, transferID)
	if err != nil {
		return err
	}
	if transfer.Status != "scheduled" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot pause transfer with status '%s'", transfer.Status)}
	}

	return s.store.UpdateScheduledTransferStatus(ctx, transferID, "paused")
}
