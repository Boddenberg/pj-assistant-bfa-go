package service

import (
	"context"
	"fmt"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// ============================================================
// Credit Cards
// ============================================================

func (s *BankingService) RequestCreditCard(ctx context.Context, customerID string, req *domain.CreditCardRequest) (*domain.CreditCard, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.RequestCreditCard")
	defer span.End()

	if req.AccountID == "" {
		return nil, &domain.ErrValidation{Field: "account_id", Message: "required"}
	}

	// Set defaults
	if req.CardBrand == "" {
		req.CardBrand = "Visa"
	}
	if req.CardType == "" {
		req.CardType = "corporate"
	}
	if req.BillingDay == 0 {
		req.BillingDay = 10
	}
	if req.DueDay == 0 {
		req.DueDay = 20
	}

	// Check account
	_, err := s.store.GetAccount(ctx, customerID, req.AccountID)
	if err != nil {
		return nil, err
	}

	card, err := s.store.CreateCreditCard(ctx, customerID, req)
	if err != nil {
		s.logger.Error("failed to create credit card", zap.String("customer_id", customerID), zap.Error(err))
		return nil, err
	}

	s.logger.Info("credit card requested",
		zap.String("customer_id", customerID),
		zap.String("card_id", card.ID),
		zap.String("brand", req.CardBrand),
	)

	return card, nil
}

func (s *BankingService) ListCreditCards(ctx context.Context, customerID string) ([]domain.CreditCard, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListCreditCards")
	defer span.End()

	return s.store.ListCreditCards(ctx, customerID)
}

func (s *BankingService) GetCreditCard(ctx context.Context, customerID, cardID string) (*domain.CreditCard, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetCreditCard")
	defer span.End()

	return s.store.GetCreditCard(ctx, customerID, cardID)
}

func (s *BankingService) ActivateCreditCard(ctx context.Context, customerID, cardID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.ActivateCreditCard")
	defer span.End()

	card, err := s.store.GetCreditCard(ctx, customerID, cardID)
	if err != nil {
		return err
	}
	if card.Status != "pending_activation" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot activate card with status '%s'", card.Status)}
	}

	return s.store.UpdateCreditCardStatus(ctx, cardID, "active")
}

func (s *BankingService) BlockCreditCard(ctx context.Context, customerID, cardID, reason string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.BlockCreditCard")
	defer span.End()

	card, err := s.store.GetCreditCard(ctx, customerID, cardID)
	if err != nil {
		return err
	}
	if card.Status != "active" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot block card with status '%s'", card.Status)}
	}

	return s.store.UpdateCreditCardStatus(ctx, cardID, "blocked")
}

func (s *BankingService) UnblockCreditCard(ctx context.Context, customerID, cardID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.UnblockCreditCard")
	defer span.End()

	card, err := s.store.GetCreditCard(ctx, customerID, cardID)
	if err != nil {
		return err
	}
	if card.Status != "blocked" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot unblock card with status '%s'", card.Status)}
	}

	return s.store.UpdateCreditCardStatus(ctx, cardID, "active")
}

// BlockCreditCardByID blocks a card using only the cardID (no customerID filter).
func (s *BankingService) BlockCreditCardByID(ctx context.Context, cardID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.BlockCreditCardByID")
	defer span.End()

	card, err := s.store.GetCreditCard(ctx, "", cardID)
	if err != nil {
		return err
	}
	if card.Status != "active" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot block card with status '%s'", card.Status)}
	}

	return s.store.UpdateCreditCardStatus(ctx, cardID, "blocked")
}

// UnblockCreditCardByID unblocks a card using only the cardID (no customerID filter).
func (s *BankingService) UnblockCreditCardByID(ctx context.Context, cardID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.UnblockCreditCardByID")
	defer span.End()

	card, err := s.store.GetCreditCard(ctx, "", cardID)
	if err != nil {
		return err
	}
	if card.Status != "blocked" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot unblock card with status '%s'", card.Status)}
	}

	return s.store.UpdateCreditCardStatus(ctx, cardID, "active")
}

// CancelScheduledTransferByID cancels a scheduled transfer using only the scheduleID.
func (s *BankingService) CancelScheduledTransferByID(ctx context.Context, scheduleID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.CancelScheduledTransferByID")
	defer span.End()

	transfer, err := s.store.GetScheduledTransfer(ctx, "", scheduleID)
	if err != nil {
		return err
	}
	if transfer.Status != "scheduled" && transfer.Status != "paused" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot cancel transfer with status '%s'", transfer.Status)}
	}

	return s.store.UpdateScheduledTransferStatus(ctx, scheduleID, "cancelled")
}

func (s *BankingService) ListCardTransactions(ctx context.Context, customerID, cardID string, page, pageSize int) ([]domain.CreditCardTransaction, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListCardTransactions")
	defer span.End()

	return s.store.ListCreditCardTransactions(ctx, customerID, cardID, page, pageSize)
}

func (s *BankingService) ListCardInvoices(ctx context.Context, customerID, cardID string) ([]domain.CreditCardInvoice, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListCardInvoices")
	defer span.End()

	return s.store.ListCreditCardInvoices(ctx, customerID, cardID)
}

func (s *BankingService) GetCardInvoice(ctx context.Context, customerID, cardID, invoiceID string) (*domain.CreditCardInvoice, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetCardInvoice")
	defer span.End()

	return s.store.GetCreditCardInvoice(ctx, customerID, cardID, invoiceID)
}

func (s *BankingService) GetCardInvoiceByMonth(ctx context.Context, customerID, cardID, month string) (*domain.CreditCardInvoice, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetCardInvoiceByMonth")
	defer span.End()

	return s.store.GetCreditCardInvoiceByMonth(ctx, customerID, cardID, month)
}

// ============================================================
// Invoice Payment
// ============================================================

// PayInvoice pays a credit card invoice (total, minimum or custom).
func (s *BankingService) PayInvoice(ctx context.Context, customerID, cardID string, req *domain.InvoicePayRequest) (*domain.InvoicePayResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.PayInvoice")
	defer span.End()
	span.SetAttributes(
		attribute.String("customer.id", customerID),
		attribute.String("card.id", cardID),
	)

	// Get current open invoice
	invoices, err := s.store.ListCreditCardInvoices(ctx, customerID, cardID)
	if err != nil {
		return nil, err
	}

	// Find the most recent open or closed (unpaid) invoice
	var targetInvoice *domain.CreditCardInvoice
	for i := range invoices {
		if invoices[i].Status == "open" || invoices[i].Status == "closed" {
			targetInvoice = &invoices[i]
			break
		}
	}
	if targetInvoice == nil {
		return nil, &domain.ErrNotFound{Resource: "invoice", ID: cardID}
	}

	// Determine amount to pay
	payAmount := req.Amount
	switch req.PaymentType {
	case "total":
		payAmount = targetInvoice.TotalAmount
	case "minimum":
		payAmount = targetInvoice.MinimumPayment
	case "custom":
		if payAmount <= 0 {
			return nil, &domain.ErrValidation{Field: "amount", Message: "valor deve ser positivo"}
		}
	default:
		return nil, &domain.ErrValidation{Field: "paymentType", Message: "deve ser total, minimum ou custom"}
	}

	// Deduct from account balance
	_, err = s.store.UpdateAccountBalance(ctx, customerID, -payAmount)
	if err != nil {
		return nil, err
	}

	// Update invoice status
	newStatus := "paid"
	if req.PaymentType == "minimum" || (req.PaymentType == "custom" && payAmount < targetInvoice.TotalAmount) {
		newStatus = "partially_paid"
	}

	err = s.store.UpdateCreditCardInvoiceStatus(ctx, targetInvoice.ID, newStatus)
	if err != nil {
		return nil, err
	}

	// Restore card available limit by the paid amount
	card, cardErr := s.store.GetCreditCard(ctx, customerID, cardID)
	if cardErr == nil {
		newUsed := card.UsedLimit - payAmount
		if newUsed < 0 {
			newUsed = 0
		}
		newAvailable := card.CreditLimit - newUsed
		if newAvailable > card.CreditLimit {
			newAvailable = card.CreditLimit
		}
		if limitErr := s.store.UpdateCreditCardUsedLimit(ctx, cardID, newUsed, newAvailable); limitErr != nil {
			s.logger.Warn("failed to restore card limit after invoice payment",
				zap.String("card_id", cardID),
				zap.Error(limitErr),
			)
		}
	}

	// Record the payment as a transaction in the statement
	now := time.Now()
	cardLast4 := cardID[:4]
	if card != nil && card.CardNumberLast4 != "" {
		cardLast4 = card.CardNumberLast4
	}
	tx := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": customerID,
		"date":        now.Format(time.RFC3339),
		"description": fmt.Sprintf("Pagamento fatura cartão •••• %s", cardLast4),
		"amount":      -payAmount,
		"type":        "bill_payment",
		"category":    "cartao",
	}
	if txErr := s.store.InsertTransaction(ctx, tx); txErr != nil {
		s.logger.Warn("failed to record invoice payment transaction", zap.Error(txErr))
	}

	s.logger.Info("invoice paid",
		zap.String("customer_id", customerID),
		zap.String("card_id", cardID),
		zap.String("invoice_id", targetInvoice.ID),
		zap.Float64("amount", payAmount),
		zap.String("payment_type", req.PaymentType),
	)

	return &domain.InvoicePayResponse{
		PaymentID:        uuid.New().String(),
		Status:           "completed",
		Amount:           payAmount,
		PaidAt:           time.Now().Format(time.RFC3339),
		NewInvoiceStatus: newStatus,
	}, nil
}
