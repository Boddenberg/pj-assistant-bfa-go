// Package service provides the business logic layer (use cases).
// BankingService handles all banking operations: PIX, transfers,
// credit cards, bill payments, analytics, etc.
package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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

// ============================================================
// PIX
// ============================================================

func (s *BankingService) ListPixKeys(ctx context.Context, customerID string) ([]domain.PixKey, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListPixKeys")
	defer span.End()

	return s.store.ListPixKeys(ctx, customerID)
}

func (s *BankingService) LookupPixKey(ctx context.Context, keyType, keyValue string) (*domain.PixKey, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.LookupPixKey")
	defer span.End()

	if keyType == "" || keyValue == "" {
		return nil, &domain.ErrValidation{Field: "key/keyType", Message: "both key and keyType are required"}
	}

	return s.store.LookupPixKey(ctx, keyType, keyValue)
}

func (s *BankingService) CreatePixTransfer(ctx context.Context, customerID string, req *domain.PixTransferRequest) (*domain.PixTransfer, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreatePixTransfer")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID), attribute.Float64("amount", req.Amount))

	start := time.Now()
	defer func() { s.metrics.RecordRequestDuration("pix_transfer", time.Since(start)) }()

	// Validate
	if req.Amount <= 0 {
		return nil, &domain.ErrValidation{Field: "amount", Message: "must be positive"}
	}
	if req.DestinationKeyValue == "" {
		return nil, &domain.ErrValidation{Field: "destination_key_value", Message: "required"}
	}
	if req.IdempotencyKey == "" {
		return nil, &domain.ErrValidation{Field: "idempotency_key", Message: "required"}
	}
	if req.SourceAccountID == "" {
		return nil, &domain.ErrValidation{Field: "source_account_id", Message: "required"}
	}
	if req.FundedBy == "" {
		req.FundedBy = "balance"
	}

	// Check account exists and belongs to customer
	account, err := s.store.GetAccount(ctx, customerID, req.SourceAccountID)
	if err != nil {
		return nil, err
	}

	// Check limits
	limit, err := s.store.GetTransactionLimit(ctx, customerID, "pix")
	if err == nil && limit != nil {
		if req.Amount > limit.SingleLimit {
			return nil, &domain.ErrLimitExceeded{LimitType: "single_pix", Limit: limit.SingleLimit, Current: req.Amount}
		}
		if limit.DailyUsed+req.Amount > limit.DailyLimit {
			return nil, &domain.ErrLimitExceeded{LimitType: "daily_pix", Limit: limit.DailyLimit, Current: limit.DailyUsed + req.Amount}
		}
	}

	// Check balance (if funded by balance)
	if req.FundedBy == "balance" && account.AvailableBalance < req.Amount {
		return nil, &domain.ErrInsufficientFunds{Available: account.AvailableBalance, Required: req.Amount}
	}

	// If funded by credit card, check credit card limit
	if req.FundedBy == "credit_card" {
		if req.CreditCardID == "" {
			return nil, &domain.ErrValidation{Field: "credit_card_id", Message: "required when funded_by is credit_card"}
		}
		card, err := s.store.GetCreditCard(ctx, customerID, req.CreditCardID)
		if err != nil {
			return nil, err
		}
		if !card.PixCreditEnabled {
			return nil, &domain.ErrValidation{Field: "credit_card_id", Message: "PIX via credit card not enabled for this card"}
		}
		if card.PixCreditUsed+req.Amount > card.PixCreditLimit {
			return nil, &domain.ErrLimitExceeded{LimitType: "pix_credit", Limit: card.PixCreditLimit, Current: card.PixCreditUsed + req.Amount}
		}
	}

	transfer, err := s.store.CreatePixTransfer(ctx, customerID, req)
	if err != nil {
		s.logger.Error("failed to create PIX transfer", zap.Error(err))
		return nil, err
	}

	s.logger.Info("PIX transfer created",
		zap.String("customer_id", customerID),
		zap.String("transfer_id", transfer.ID),
		zap.Float64("amount", req.Amount),
		zap.String("funded_by", req.FundedBy),
	)

	return transfer, nil
}

func (s *BankingService) ListPixTransfers(ctx context.Context, customerID string, page, pageSize int) ([]domain.PixTransfer, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListPixTransfers")
	defer span.End()

	return s.store.ListPixTransfers(ctx, customerID, page, pageSize)
}

func (s *BankingService) GetPixTransfer(ctx context.Context, customerID, transferID string) (*domain.PixTransfer, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetPixTransfer")
	defer span.End()

	return s.store.GetPixTransfer(ctx, customerID, transferID)
}

func (s *BankingService) CancelPixTransfer(ctx context.Context, customerID, transferID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.CancelPixTransfer")
	defer span.End()

	transfer, err := s.store.GetPixTransfer(ctx, customerID, transferID)
	if err != nil {
		return err
	}

	if transfer.Status != "pending" && transfer.Status != "scheduled" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot cancel transfer with status '%s'", transfer.Status)}
	}

	return s.store.UpdatePixTransferStatus(ctx, transferID, "cancelled")
}

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

	return s.store.CreateScheduledTransfer(ctx, customerID, req)
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

	return s.store.CreateCreditCard(ctx, customerID, req)
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

	// Pass empty customerID — the store will match by cardID only
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
// Bill Payments
// ============================================================

var digitOnlyRegex = regexp.MustCompile(`[^0-9]`)

// ValidateBarcode validates a barcode or digitable line.
func (s *BankingService) ValidateBarcode(ctx context.Context, req *domain.BarcodeValidationRequest) (*domain.BarcodeValidationResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ValidateBarcode")
	defer span.End()

	resp := &domain.BarcodeValidationResponse{}

	// Determine what we're validating
	input := req.DigitableLine
	if input == "" {
		input = req.Barcode
	}
	if input == "" {
		return nil, &domain.ErrValidation{Field: "digitable_line|barcode", Message: "at least one is required"}
	}

	// Clean: keep only digits
	clean := digitOnlyRegex.ReplaceAllString(input, "")

	switch len(clean) {
	case 47:
		// Boleto bancário
		resp.IsValid = true
		resp.BillType = "bank_slip"
		resp.DigitableLine = clean
		resp.BankCode = clean[:3]
		// Extract amount from positions 37-47
		amtRaw := clean[37:47]
		if amt, err := strconv.ParseFloat(amtRaw, 64); err == nil {
			resp.Amount = amt / 100 // centavos → reais
		}
		// Due date factor (positions 33-37)
		dueFactor := clean[33:37]
		if factor, err := strconv.Atoi(dueFactor); err == nil && factor > 0 {
			baseDate := time.Date(1997, 10, 7, 0, 0, 0, 0, time.UTC)
			dueDate := baseDate.AddDate(0, 0, factor)
			resp.DueDate = dueDate.Format("2006-01-02")
		}

	case 48:
		// Concessionária / utility
		resp.IsValid = true
		resp.BillType = "utility"
		resp.DigitableLine = clean
		// Segment identifier
		segID := clean[:1]
		if segID == "8" {
			resp.BillType = "utility"
		}
		// Amount from positions 4-15
		amtRaw := clean[4:15]
		if amt, err := strconv.ParseFloat(amtRaw, 64); err == nil {
			resp.Amount = amt / 100
		}

	case 44:
		// Barcode (not digitable line)
		resp.IsValid = true
		resp.BillType = "bank_slip"
		resp.Barcode = clean
		resp.BankCode = clean[:3]

	default:
		resp.IsValid = false
		resp.ValidationErrors = []string{
			fmt.Sprintf("input has %d digits, expected 44 (barcode), 47 (boleto) or 48 (concessionária)", len(clean)),
		}
	}

	return resp, nil
}

func (s *BankingService) PayBill(ctx context.Context, customerID string, req *domain.BillPaymentRequest) (*domain.BillPayment, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.PayBill")
	defer span.End()

	start := time.Now()
	defer func() { s.metrics.RecordRequestDuration("bill_payment", time.Since(start)) }()

	if req.IdempotencyKey == "" {
		return nil, &domain.ErrValidation{Field: "idempotency_key", Message: "required"}
	}
	if req.AccountID == "" {
		return nil, &domain.ErrValidation{Field: "account_id", Message: "required"}
	}

	// Validate the barcode/digitable line
	valReq := &domain.BarcodeValidationRequest{
		InputMethod:   req.InputMethod,
		Barcode:       req.Barcode,
		DigitableLine: req.DigitableLine,
	}
	valResult, err := s.ValidateBarcode(ctx, valReq)
	if err != nil {
		return nil, err
	}
	if !valResult.IsValid {
		return nil, &domain.ErrInvalidBarcode{
			Input:  req.DigitableLine + req.Barcode,
			Reason: strings.Join(valResult.ValidationErrors, "; "),
		}
	}

	// Check account & balance
	account, err := s.store.GetAccount(ctx, customerID, req.AccountID)
	if err != nil {
		return nil, err
	}

	amount := req.Amount
	if amount == 0 {
		amount = valResult.Amount
	}

	if account.AvailableBalance < amount {
		return nil, &domain.ErrInsufficientFunds{Available: account.AvailableBalance, Required: amount}
	}

	// Check limit
	limit, err := s.store.GetTransactionLimit(ctx, customerID, "bill_payment")
	if err == nil && limit != nil {
		if amount > limit.SingleLimit {
			return nil, &domain.ErrLimitExceeded{LimitType: "single_bill", Limit: limit.SingleLimit, Current: amount}
		}
	}

	return s.store.CreateBillPayment(ctx, customerID, req, valResult)
}

func (s *BankingService) ListBillPayments(ctx context.Context, customerID string, page, pageSize int) ([]domain.BillPayment, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListBillPayments")
	defer span.End()

	return s.store.ListBillPayments(ctx, customerID, page, pageSize)
}

func (s *BankingService) GetBillPayment(ctx context.Context, customerID, billID string) (*domain.BillPayment, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetBillPayment")
	defer span.End()

	return s.store.GetBillPayment(ctx, customerID, billID)
}

func (s *BankingService) CancelBillPayment(ctx context.Context, customerID, billID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.CancelBillPayment")
	defer span.End()

	bill, err := s.store.GetBillPayment(ctx, customerID, billID)
	if err != nil {
		return err
	}
	if bill.Status != "pending" && bill.Status != "scheduled" && bill.Status != "validated" {
		return &domain.ErrValidation{Field: "status", Message: fmt.Sprintf("cannot cancel bill with status '%s'", bill.Status)}
	}

	return s.store.UpdateBillPaymentStatus(ctx, billID, "cancelled")
}

// ============================================================
// Debit Purchases
// ============================================================

func (s *BankingService) ListDebitPurchases(ctx context.Context, customerID string, page, pageSize int) ([]domain.DebitPurchase, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListDebitPurchases")
	defer span.End()

	return s.store.ListDebitPurchases(ctx, customerID, page, pageSize)
}

func (s *BankingService) CreateDebitPurchase(ctx context.Context, customerID string, req *domain.DebitPurchaseRequest) (*domain.DebitPurchaseResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreateDebitPurchase")
	defer span.End()

	if req.Amount <= 0 {
		return nil, &domain.ErrValidation{Field: "amount", Message: "must be positive"}
	}
	if req.MerchantName == "" {
		return nil, &domain.ErrValidation{Field: "merchantName", Message: "required"}
	}

	// Get primary account and check balance
	account, err := s.store.GetPrimaryAccount(ctx, customerID)
	if err != nil {
		return nil, err
	}

	if account.AvailableBalance < req.Amount {
		return &domain.DebitPurchaseResponse{
			Status:    "insufficient_funds",
			Amount:    req.Amount,
			Timestamp: time.Now().Format(time.RFC3339),
		}, nil
	}

	purchase, err := s.store.CreateDebitPurchase(ctx, customerID, req)
	if err != nil {
		return nil, err
	}

	return &domain.DebitPurchaseResponse{
		TransactionID: purchase.ID,
		Status:        "completed",
		Amount:        purchase.Amount,
		NewBalance:    account.AvailableBalance - purchase.Amount,
		Timestamp:     purchase.TransactionDate.Format(time.RFC3339),
	}, nil
}

// ============================================================
// Spending Analytics
// ============================================================

func (s *BankingService) GetSpendingSummary(ctx context.Context, customerID, periodType string) (*domain.SpendingSummary, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetSpendingSummary")
	defer span.End()

	return s.store.GetSpendingSummary(ctx, customerID, periodType)
}

func (s *BankingService) GetCategoryBreakdown(ctx context.Context, customerID, periodType string) (map[string]domain.CatSum, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetCategoryBreakdown")
	defer span.End()

	summary, err := s.store.GetSpendingSummary(ctx, customerID, periodType)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return map[string]domain.CatSum{}, nil
	}
	return summary.CategoryBreakdown, nil
}

func (s *BankingService) ListBudgets(ctx context.Context, customerID string) ([]domain.SpendingBudget, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListBudgets")
	defer span.End()

	return s.store.ListBudgets(ctx, customerID)
}

func (s *BankingService) CreateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreateBudget")
	defer span.End()

	if budget.Category == "" {
		return nil, &domain.ErrValidation{Field: "category", Message: "required"}
	}
	if budget.MonthlyLimit <= 0 {
		return nil, &domain.ErrValidation{Field: "monthly_limit", Message: "must be positive"}
	}
	if budget.AlertThresholdPct == 0 {
		budget.AlertThresholdPct = 80.0
	}
	budget.IsActive = true

	return s.store.CreateBudget(ctx, budget)
}

func (s *BankingService) UpdateBudget(ctx context.Context, budget *domain.SpendingBudget) (*domain.SpendingBudget, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.UpdateBudget")
	defer span.End()

	return s.store.UpdateBudget(ctx, budget)
}

// ============================================================
// Favorites
// ============================================================

func (s *BankingService) ListFavorites(ctx context.Context, customerID string) ([]domain.Favorite, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListFavorites")
	defer span.End()

	return s.store.ListFavorites(ctx, customerID)
}

func (s *BankingService) CreateFavorite(ctx context.Context, fav *domain.Favorite) (*domain.Favorite, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreateFavorite")
	defer span.End()

	if fav.Nickname == "" {
		return nil, &domain.ErrValidation{Field: "nickname", Message: "required"}
	}
	if fav.RecipientName == "" {
		return nil, &domain.ErrValidation{Field: "recipient_name", Message: "required"}
	}

	return s.store.CreateFavorite(ctx, fav)
}

func (s *BankingService) DeleteFavorite(ctx context.Context, customerID, favoriteID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.DeleteFavorite")
	defer span.End()

	return s.store.DeleteFavorite(ctx, customerID, favoriteID)
}

// ============================================================
// Transaction Limits
// ============================================================

func (s *BankingService) ListLimits(ctx context.Context, customerID string) ([]domain.TransactionLimit, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListLimits")
	defer span.End()

	return s.store.ListTransactionLimits(ctx, customerID)
}

func (s *BankingService) UpdateLimit(ctx context.Context, limit *domain.TransactionLimit) (*domain.TransactionLimit, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.UpdateLimit")
	defer span.End()

	return s.store.UpdateTransactionLimit(ctx, limit)
}

// ============================================================
// Notifications
// ============================================================

func (s *BankingService) ListNotifications(ctx context.Context, customerID string, unreadOnly bool, page, pageSize int) ([]domain.Notification, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.ListNotifications")
	defer span.End()

	return s.store.ListNotifications(ctx, customerID, unreadOnly, page, pageSize)
}

func (s *BankingService) MarkNotificationRead(ctx context.Context, notifID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.MarkNotificationRead")
	defer span.End()

	return s.store.MarkNotificationRead(ctx, notifID)
}

// ============================================================
// Financial Summary (aggregated view for the frontend spec)
// ============================================================

func (s *BankingService) GetFinancialSummary(ctx context.Context, customerID, period string) (*domain.FinancialSummary, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetFinancialSummary")
	defer span.End()

	// Get account balance
	account, err := s.store.GetPrimaryAccount(ctx, customerID)
	if err != nil {
		s.logger.Warn("no account for financial summary", zap.String("customer_id", customerID), zap.Error(err))
		account = &domain.Account{}
	}

	// Determine period label and dates
	now := time.Now()
	periodLabel := "Últimos 30 dias"
	periodDays := 30
	switch {
	case period == "90d":
		periodLabel = "Últimos 90 dias"
		periodDays = 90
	case period == "12m":
		periodLabel = "Últimos 12 meses"
		periodDays = 365
	case period == "7d":
		periodLabel = "Últimos 7 dias"
		periodDays = 7
	}
	fromDate := now.AddDate(0, 0, -periodDays).Format("2006-01-02")
	toDate := now.Format("2006-01-02")

	// Get spending summary from the store
	spendingSummary, err := s.store.GetSpendingSummary(ctx, customerID, "monthly")
	if err != nil {
		s.logger.Warn("no spending summary", zap.String("customer_id", customerID), zap.Error(err))
		spendingSummary = &domain.SpendingSummary{}
	}

	// Build top categories
	topCategories := make([]domain.TopCategory, 0)
	if spendingSummary.CategoryBreakdown != nil {
		for cat, cs := range spendingSummary.CategoryBreakdown {
			topCategories = append(topCategories, domain.TopCategory{
				Category:         cat,
				Amount:           cs.Total,
				Percentage:       cs.Pct,
				TransactionCount: cs.Count,
				Trend:            "stable",
			})
		}
	}

	avgDaily := float64(0)
	if periodDays > 0 && spendingSummary.TotalExpenses > 0 {
		avgDaily = spendingSummary.TotalExpenses / float64(periodDays)
	}

	return &domain.FinancialSummary{
		CustomerID: customerID,
		Period: &domain.FinancialPeriod{
			From:  fromDate,
			To:    toDate,
			Label: periodLabel,
		},
		Balance: &domain.BalanceSummary{
			Current:   account.Balance,
			Available: account.AvailableBalance,
			Blocked:   account.Balance - account.AvailableBalance,
			Invested:  0,
		},
		CashFlow: &domain.CashFlowSummary{
			TotalIncome:              spendingSummary.TotalIncome,
			TotalExpenses:            spendingSummary.TotalExpenses,
			NetCashFlow:              spendingSummary.NetCashflow,
			ComparedToPreviousPeriod: spendingSummary.IncomeVariationPct,
		},
		Spending: &domain.SpendingDetail{
			TotalSpent:               spendingSummary.TotalExpenses,
			AverageDaily:             avgDaily,
			ComparedToPreviousPeriod: spendingSummary.ExpenseVariationPct,
		},
		TopCategories: topCategories,
		MonthlyTrend:  []domain.MonthlyTrend{},
	}, nil
}

// GetTransactionSummary computes an aggregated summary of customer transactions.
func (s *BankingService) GetTransactionSummary(ctx context.Context, customerID string) (*domain.TransactionSummary, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetTransactionSummary")
	defer span.End()

	return s.store.GetTransactionSummary(ctx, customerID)
}
