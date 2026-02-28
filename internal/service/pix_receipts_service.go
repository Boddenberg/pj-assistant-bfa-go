package service

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
)

// ============================================================
// PIX Receipts (Comprovantes)
// ============================================================

func (s *BankingService) GetPixReceipt(ctx context.Context, receiptID string) (*domain.PixReceipt, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetPixReceipt")
	defer span.End()

	return s.store.GetPixReceipt(ctx, receiptID)
}

func (s *BankingService) GetPixReceiptByTransferID(ctx context.Context, transferID string) (*domain.PixReceipt, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetPixReceiptByTransferID")
	defer span.End()

	return s.store.GetPixReceiptByTransferID(ctx, transferID)
}

func (s *BankingService) ListPixReceipts(ctx context.Context, customerID string) ([]domain.PixReceipt, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListPixReceipts")
	defer span.End()

	return s.store.ListPixReceipts(ctx, customerID)
}
