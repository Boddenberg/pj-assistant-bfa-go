package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

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

	bill, err := s.store.CreateBillPayment(ctx, customerID, req, valResult)
	if err != nil {
		s.logger.Error("failed to create bill payment", zap.String("customer_id", customerID), zap.Error(err))
		return nil, err
	}

	// Debit account balance
	if _, balErr := s.store.UpdateAccountBalance(ctx, customerID, -amount); balErr != nil {
		s.logger.Error("failed to debit balance after bill payment",
			zap.String("customer_id", customerID),
			zap.Error(balErr),
		)
	}

	// Record in customer_transactions
	now := time.Now()
	desc := fmt.Sprintf("Pagamento de boleto - %s", valResult.BillType)
	if valResult.BeneficiaryName != "" {
		desc = fmt.Sprintf("Pagamento de boleto - %s", valResult.BeneficiaryName)
	}
	txRec := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": customerID,
		"date":        now.Format(time.RFC3339),
		"description": desc,
		"amount":      -amount,
		"type":        "bill_payment",
		"category":    "contas",
	}
	if txErr := s.store.InsertTransaction(ctx, txRec); txErr != nil {
		s.logger.Error("failed to record bill transaction",
			zap.String("customer_id", customerID),
			zap.Error(txErr),
		)
	}

	s.logger.Info("bill payment created",
		zap.String("customer_id", customerID),
		zap.String("bill_id", bill.ID),
		zap.Float64("amount", amount),
		zap.String("bill_type", valResult.BillType),
	)

	return bill, nil
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
		s.logger.Error("failed to create debit purchase", zap.String("customer_id", customerID), zap.Error(err))
		return nil, err
	}

	// Debit account balance
	updatedAcct, balErr := s.store.UpdateAccountBalance(ctx, customerID, -purchase.Amount)
	newBalance := account.AvailableBalance - purchase.Amount
	if balErr != nil {
		s.logger.Error("failed to debit balance after debit purchase",
			zap.String("customer_id", customerID),
			zap.Error(balErr),
		)
	} else {
		newBalance = updatedAcct.AvailableBalance
	}

	// Record in customer_transactions
	now := time.Now()
	txRec := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": customerID,
		"date":        now.Format(time.RFC3339),
		"description": fmt.Sprintf("Compra débito - %s", req.MerchantName),
		"amount":      -purchase.Amount,
		"type":        "debit_purchase",
		"category":    "compras",
	}
	if txErr := s.store.InsertTransaction(ctx, txRec); txErr != nil {
		s.logger.Error("failed to record debit purchase transaction",
			zap.String("customer_id", customerID),
			zap.Error(txErr),
		)
	}

	s.logger.Info("debit purchase completed",
		zap.String("customer_id", customerID),
		zap.String("transaction_id", purchase.ID),
		zap.Float64("amount", purchase.Amount),
		zap.String("merchant", req.MerchantName),
	)

	return &domain.DebitPurchaseResponse{
		TransactionID: purchase.ID,
		Status:        "completed",
		Amount:        purchase.Amount,
		NewBalance:    newBalance,
		Timestamp:     purchase.TransactionDate.Format(time.RFC3339),
	}, nil
}
