// Package service provides the business logic layer (use cases).
// BankingService handles all banking operations: PIX, transfers,
// credit cards, bill payments, analytics, etc.
package service

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"

	"github.com/google/uuid"
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

	if keyValue == "" {
		return nil, &domain.ErrValidation{Field: "key", Message: "key is required"}
	}

	// Auto-detect keyType from value format if not provided
	if keyType == "" {
		keyType = detectPixKeyType(keyValue)
	}

	// If we have a keyType, search with it; otherwise search by value only
	if keyType != "" {
		return s.store.LookupPixKey(ctx, keyType, keyValue)
	}
	return s.store.LookupPixKeyByValue(ctx, keyValue)
}

// detectPixKeyType infers the pix key type from the value format.
func detectPixKeyType(value string) string {
	// Strip non-digit chars for numeric checks
	digits := ""
	hasCNPJFormatting := false
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
		if r == '.' || r == '/' {
			hasCNPJFormatting = true
		}
	}

	// Email — check first since it's unambiguous
	if strings.Contains(value, "@") {
		return "email"
	}
	// UUID-like → random
	if len(value) == 36 && strings.Count(value, "-") == 4 {
		return "random"
	}
	// CNPJ: 14 digits, or 11-14 digits with CNPJ formatting (dots/slashes)
	if len(digits) == 14 {
		return "cnpj"
	}
	if hasCNPJFormatting && len(digits) >= 11 && len(digits) <= 14 {
		return "cnpj"
	}
	// CPF: 11 digits (not starting with +)
	if len(digits) == 11 && !strings.HasPrefix(value, "+") {
		return "cpf"
	}
	// Phone: starts with + or has 10-13 digits (only if no CNPJ formatting)
	if strings.HasPrefix(value, "+") {
		return "phone"
	}
	if len(digits) >= 10 && len(digits) <= 13 && !hasCNPJFormatting {
		return "phone"
	}
	// Could not determine
	return ""
}

// GetCustomerName resolves a customer ID to a human-readable name.
func (s *BankingService) GetCustomerName(ctx context.Context, customerID string) (string, error) {
	return s.store.GetCustomerName(ctx, customerID)
}

// GetCustomerLookupData returns full profile + account data for pix lookup responses.
func (s *BankingService) GetCustomerLookupData(ctx context.Context, customerID string) (name, document, bank, branch, account string, err error) {
	return s.store.GetCustomerLookupData(ctx, customerID)
}

// DeletePixKey removes a Pix key for the given customer.
func (s *BankingService) DeletePixKey(ctx context.Context, customerID, keyID string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.DeletePixKey")
	defer span.End()

	if customerID == "" || keyID == "" {
		return &domain.ErrValidation{Field: "keyId", Message: "required"}
	}

	err := s.store.DeletePixKey(ctx, customerID, keyID)
	if err != nil {
		s.logger.Error("failed to delete pix key",
			zap.String("customer_id", customerID),
			zap.String("key_id", keyID),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("pix key deleted",
		zap.String("customer_id", customerID),
		zap.String("key_id", keyID),
	)
	return nil
}

// DeletePixKeyByValue removes a Pix key by its type and value.
func (s *BankingService) DeletePixKeyByValue(ctx context.Context, customerID, keyType, keyValue string) error {
	ctx, span := bankTracer.Start(ctx, "BankingService.DeletePixKeyByValue")
	defer span.End()

	// Lookup the key to get its ID
	key, err := s.store.LookupPixKey(ctx, keyType, keyValue)
	if err != nil {
		return err
	}
	// Verify it belongs to the customer
	if key.CustomerID != customerID {
		return &domain.ErrNotFound{Resource: "pix_key", ID: keyValue}
	}

	return s.store.DeletePixKey(ctx, customerID, key.ID)
}

// GetCreditLimit returns the total credit limit for a customer's cards.
func (s *BankingService) GetCreditLimit(ctx context.Context, customerID string) (float64, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetCreditLimit")
	defer span.End()

	cards, err := s.store.ListCreditCards(ctx, customerID)
	if err != nil {
		return 0, err
	}
	if len(cards) == 0 {
		return 0, &domain.ErrNotFound{Resource: "credit_card", ID: customerID}
	}

	// Return the highest credit limit among all cards
	var maxLimit float64
	for _, c := range cards {
		if c.CreditLimit > maxLimit {
			maxLimit = c.CreditLimit
		}
	}
	return maxLimit, nil
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

	// Block self-transfer: look up destination key (uses auto-detect if keyType is empty)
	destKey, lookupErr := s.LookupPixKey(ctx, req.DestinationKeyType, req.DestinationKeyValue)
	if lookupErr == nil && destKey != nil && destKey.CustomerID == customerID {
		return nil, &domain.ErrValidation{Field: "recipientKey", Message: "Não é possível transferir para você mesmo"}
	}

	// Auto-detect destination key type if not provided
	if req.DestinationKeyType == "" {
		detected := detectPixKeyType(req.DestinationKeyValue)
		if detected != "" {
			req.DestinationKeyType = detected
		} else {
			req.DestinationKeyType = "manual"
		}
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
		// Calculate total with fees if not already set
		if req.TotalWithFees <= 0 {
			installments := req.CreditCardInstallments
			if installments <= 0 {
				installments = 1
			}
			feeRate := req.FeeRate
			if feeRate <= 0 {
				feeRate = 0.02
			}
			req.TotalWithFees = req.Amount * (1 + feeRate*float64(installments-1))
		}
		// Validate against TOTAL with fees, not just raw amount
		if card.PixCreditUsed+req.TotalWithFees > card.PixCreditLimit {
			return nil, &domain.ErrLimitExceeded{LimitType: "pix_credit", Limit: card.PixCreditLimit, Current: card.PixCreditUsed + req.TotalWithFees}
		}
	}

	// ── Resolve destination info (name, document) from pix key owner ──
	var destCustomerID string
	if destKey != nil {
		destCustomerID = destKey.CustomerID
		destName, destDoc, _, _, _, lookupErr := s.store.GetCustomerLookupData(ctx, destKey.CustomerID)
		if lookupErr == nil {
			req.DestinationName = destName
			req.DestinationDocument = destDoc
		}
	}

	// ── Sender name for destination extrato ──
	senderName, _ := s.store.GetCustomerName(ctx, customerID)
	if senderName == "" || senderName == "Destinatário" {
		senderName = "Remetente"
	}
	// Sender full lookup data for receipts
	senderDoc, senderBank, senderBranch, senderAcct := "", "", "", ""
	if sName, sDoc, sBank, sBranch, sAcct, sErr := s.store.GetCustomerLookupData(ctx, customerID); sErr == nil {
		if senderName == "Remetente" {
			senderName = sName
		}
		senderDoc = sDoc
		senderBank = sBank
		senderBranch = sBranch
		senderAcct = sAcct
	}
	// Destination full lookup data for receipts
	destBank, destBranch, destAcct := "", "", ""
	if destCustomerID != "" {
		if _, _, dBank, dBranch, dAcct, dErr := s.store.GetCustomerLookupData(ctx, destCustomerID); dErr == nil {
			destBank = dBank
			destBranch = dBranch
			destAcct = dAcct
		}
	}

	transfer, err := s.store.CreatePixTransfer(ctx, customerID, req)
	if err != nil {
		s.logger.Error("failed to create PIX transfer", zap.Error(err))
		return nil, err
	}

	now := time.Now()

	// ── 1. Debit sender ──
	descSent := fmt.Sprintf("Pix enviado - %s", transfer.DestinationKeyValue)
	if transfer.DestinationName != "" {
		descSent = fmt.Sprintf("Pix enviado - %s", transfer.DestinationName)
	}

	if req.FundedBy == "credit_card" {
		// ── 1a. Credit card: debit card limit + record in fatura ──
		card, _ := s.store.GetCreditCard(ctx, customerID, req.CreditCardID)
		if card != nil {
			// Debit the TOTAL with fees from the card limit
			debitAmount := req.TotalWithFees
			if debitAmount <= 0 {
				debitAmount = req.Amount
			}
			newUsed := card.UsedLimit + debitAmount
			newAvailable := card.CreditLimit - newUsed
			if newAvailable < 0 {
				newAvailable = 0
			}
			if ulErr := s.store.UpdateCreditCardUsedLimit(ctx, card.ID, newUsed, newAvailable); ulErr != nil {
				s.logger.Error("failed to update card used_limit after pix credit",
					zap.String("card_id", card.ID), zap.Error(ulErr))
			}
			// Also update pix_credit_used on the card
			newPixUsed := card.PixCreditUsed + debitAmount
			if pxErr := s.store.UpdateCreditCardPixCreditUsed(ctx, card.ID, newPixUsed); pxErr != nil {
				s.logger.Error("failed to update pix_credit_used after pix credit",
					zap.String("card_id", card.ID), zap.Error(pxErr))
			}
		}

		// Insert into credit_card_transactions (fatura do remetente)
		faturaAmount := req.TotalWithFees
		if faturaAmount <= 0 {
			faturaAmount = req.Amount
		}
		ccTx := map[string]any{
			"id":                  uuid.New().String(),
			"card_id":             req.CreditCardID,
			"customer_id":         customerID,
			"transaction_date":    now.Format(time.RFC3339),
			"amount":              faturaAmount,
			"merchant_name":       descSent,
			"category":            "pix_credito",
			"description":         descSent,
			"installments":        req.CreditCardInstallments,
			"current_installment": 1,
			"transaction_type":    "pix_credit",
			"status":              "confirmed",
		}
		if req.CreditCardInstallments <= 0 {
			ccTx["installments"] = 1
		}
		if txErr := s.store.InsertCreditCardTransaction(ctx, ccTx); txErr != nil {
			s.logger.Error("failed to record pix credit card transaction in fatura",
				zap.String("customer_id", customerID), zap.Error(txErr))
		}
	} else {
		// ── 1b. Balance: debit account + record pix_sent in extrato ──
		if _, balErr := s.store.UpdateAccountBalance(ctx, customerID, -req.Amount); balErr != nil {
			s.logger.Error("failed to debit sender balance after pix transfer",
				zap.String("customer_id", customerID),
				zap.Error(balErr),
			)
		}

		txSent := map[string]any{
			"id":          uuid.New().String(),
			"customer_id": customerID,
			"date":        now.Format(time.RFC3339),
			"description": descSent,
			"amount":      -req.Amount,
			"type":        "pix_sent",
			"category":    "pix",
		}
		if txErr := s.store.InsertTransaction(ctx, txSent); txErr != nil {
			s.logger.Error("failed to record sender pix transaction",
				zap.String("customer_id", customerID),
				zap.Error(txErr),
			)
		}
	}

	// ── 2. Credit destination balance and record pix_received transaction ──
	if destCustomerID != "" {
		// Credit destination account
		if _, balErr := s.store.UpdateAccountBalance(ctx, destCustomerID, req.Amount); balErr != nil {
			s.logger.Error("failed to credit destination balance after pix transfer",
				zap.String("dest_customer_id", destCustomerID),
				zap.Error(balErr),
			)
		} else {
			s.logger.Info("PIX destination credited",
				zap.String("dest_customer_id", destCustomerID),
				zap.Float64("amount", req.Amount),
			)
		}

		// Record pix_received in destination's extrato
		txReceived := map[string]any{
			"id":          uuid.New().String(),
			"customer_id": destCustomerID,
			"date":        now.Format(time.RFC3339),
			"description": fmt.Sprintf("Pix recebido - %s", senderName),
			"amount":      req.Amount,
			"type":        "pix_received",
			"category":    "recebimento",
		}
		if txErr := s.store.InsertTransaction(ctx, txReceived); txErr != nil {
			s.logger.Error("failed to record destination pix_received transaction",
				zap.String("dest_customer_id", destCustomerID),
				zap.Error(txErr),
			)
		}
	}

	// ── 3. Mark transfer as completed ──
	if updErr := s.store.UpdatePixTransferStatus(ctx, transfer.ID, "completed"); updErr != nil {
		s.logger.Error("failed to update pix transfer status to completed",
			zap.String("transfer_id", transfer.ID),
			zap.Error(updErr),
		)
	} else {
		transfer.Status = "completed"
	}

	// ── 4. Save PIX receipt (comprovante) for sender ──
	nowStr := now.Format(time.RFC3339)
	installments := req.CreditCardInstallments
	if installments <= 0 {
		installments = 1
	}
	// Compute fee fields for receipt
	feeAmount := 0.0
	totalAmount := req.Amount
	if req.FundedBy == "credit_card" && req.TotalWithFees > 0 {
		feeAmount = req.TotalWithFees - req.Amount
		totalAmount = req.TotalWithFees
	}

	receiptSent := &domain.PixReceipt{
		ID:                uuid.New().String(),
		TransferID:        transfer.ID,
		CustomerID:        customerID,
		Direction:         "sent",
		Amount:            req.Amount,
		OriginalAmount:    req.Amount,
		FeeAmount:         feeAmount,
		TotalAmount:       totalAmount,
		Description:       req.Description,
		EndToEndID:        transfer.EndToEndID,
		FundedBy:          req.FundedBy,
		Installments:      installments,
		SenderName:        senderName,
		SenderDocument:    senderDoc,
		SenderBank:        senderBank,
		SenderBranch:      senderBranch,
		SenderAccount:     senderAcct,
		RecipientName:     transfer.DestinationName,
		RecipientDocument: transfer.DestinationDocument,
		RecipientBank:     destBank,
		RecipientBranch:   destBranch,
		RecipientAccount:  destAcct,
		RecipientKeyType:  transfer.DestinationKeyType,
		RecipientKeyValue: transfer.DestinationKeyValue,
		Status:            "completed",
		ExecutedAt:        nowStr,
		CreatedAt:         nowStr,
	}
	if savedReceipt, rcptErr := s.store.SavePixReceipt(ctx, receiptSent); rcptErr != nil {
		s.logger.Error("failed to save pix receipt for sender",
			zap.String("transfer_id", transfer.ID), zap.Error(rcptErr))
	} else {
		transfer.ReceiptID = savedReceipt.ID
	}

	// ── 5. Save PIX receipt for destination (if internal) ──
	if destCustomerID != "" {
		receiptReceived := &domain.PixReceipt{
			ID:                uuid.New().String(),
			TransferID:        transfer.ID,
			CustomerID:        destCustomerID,
			Direction:         "received",
			Amount:            req.Amount,
			OriginalAmount:    req.Amount,
			FeeAmount:         0,
			TotalAmount:       req.Amount,
			Description:       req.Description,
			EndToEndID:        transfer.EndToEndID,
			FundedBy:          req.FundedBy,
			SenderName:        senderName,
			SenderDocument:    senderDoc,
			SenderBank:        senderBank,
			SenderBranch:      senderBranch,
			SenderAccount:     senderAcct,
			RecipientName:     transfer.DestinationName,
			RecipientDocument: transfer.DestinationDocument,
			RecipientBank:     destBank,
			RecipientBranch:   destBranch,
			RecipientAccount:  destAcct,
			RecipientKeyType:  transfer.DestinationKeyType,
			RecipientKeyValue: transfer.DestinationKeyValue,
			Status:            "completed",
			ExecutedAt:        nowStr,
			CreatedAt:         nowStr,
		}
		if _, rcptErr := s.store.SavePixReceipt(ctx, receiptReceived); rcptErr != nil {
			s.logger.Error("failed to save pix receipt for destination",
				zap.String("dest_customer_id", destCustomerID), zap.Error(rcptErr))
		}
	}

	s.logger.Info("PIX transfer completed",
		zap.String("customer_id", customerID),
		zap.String("dest_customer_id", destCustomerID),
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
	switch period {
	case "7d":
		periodLabel = "Últimos 7 dias"
		periodDays = 7
	case "90d", "3months":
		periodLabel = "Últimos 3 meses"
		periodDays = 90
	case "6months":
		periodLabel = "Últimos 6 meses"
		periodDays = 180
	case "12m", "1year":
		periodLabel = "Últimos 12 meses"
		periodDays = 365
	case "1month", "30d":
		periodLabel = "Últimos 30 dias"
		periodDays = 30
	}
	fromDate := now.AddDate(0, 0, -periodDays).Format("2006-01-02")
	toDate := now.Format("2006-01-02")

	// Fetch actual transactions from customer_transactions
	txns, txErr := s.store.ListTransactions(ctx, customerID, fromDate, toDate)
	if txErr != nil {
		s.logger.Warn("could not list transactions for financial summary", zap.Error(txErr))
		txns = nil
	}

	// Compute income, expenses, and category breakdown from real transactions
	var totalIncome, totalExpenses float64
	categoryMap := make(map[string]struct {
		Total float64
		Count int
	})

	// Monthly breakdown for trend
	monthlyIncome := make(map[string]float64)
	monthlyExpenses := make(map[string]float64)

	for _, tx := range txns {
		monthKey := tx.Date.Format("2006-01")
		if tx.Amount >= 0 {
			totalIncome += tx.Amount
			monthlyIncome[monthKey] += tx.Amount
		} else {
			totalExpenses += -tx.Amount // store as positive for display
			monthlyExpenses[monthKey] += -tx.Amount
		}
		if tx.Category != "" {
			entry := categoryMap[tx.Category]
			entry.Total += -tx.Amount // positive value for expense categories
			if tx.Amount < 0 {
				entry.Count++
			}
			categoryMap[tx.Category] = entry
		}
	}

	// Build top categories
	topCategories := make([]domain.TopCategory, 0)
	for cat, info := range categoryMap {
		if info.Total <= 0 {
			continue // skip income categories
		}
		pct := float64(0)
		if totalExpenses > 0 {
			pct = (info.Total / totalExpenses) * 100
		}
		topCategories = append(topCategories, domain.TopCategory{
			Category:         cat,
			Amount:           info.Total,
			Percentage:       pct,
			TransactionCount: info.Count,
			Trend:            "stable",
		})
	}

	// Build monthly trend
	monthlyTrend := make([]domain.MonthlyTrend, 0)
	// Collect all months
	monthSet := make(map[string]bool)
	for m := range monthlyIncome {
		monthSet[m] = true
	}
	for m := range monthlyExpenses {
		monthSet[m] = true
	}
	for m := range monthSet {
		inc := monthlyIncome[m]
		exp := monthlyExpenses[m]
		monthlyTrend = append(monthlyTrend, domain.MonthlyTrend{
			Month:    m,
			Income:   inc,
			Expenses: exp,
			Balance:  inc - exp,
		})
	}

	// Sort monthly trend by month ascending
	for i := 0; i < len(monthlyTrend); i++ {
		for j := i + 1; j < len(monthlyTrend); j++ {
			if monthlyTrend[i].Month > monthlyTrend[j].Month {
				monthlyTrend[i], monthlyTrend[j] = monthlyTrend[j], monthlyTrend[i]
			}
		}
	}

	netCashFlow := totalIncome - totalExpenses
	avgDaily := float64(0)
	if periodDays > 0 && totalExpenses > 0 {
		avgDaily = totalExpenses / float64(periodDays)
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
			TotalIncome:              totalIncome,
			TotalExpenses:            totalExpenses,
			NetCashFlow:              netCashFlow,
			ComparedToPreviousPeriod: 0,
		},
		Spending: &domain.SpendingDetail{
			TotalSpent:               totalExpenses,
			AverageDaily:             avgDaily,
			ComparedToPreviousPeriod: 0,
		},
		TopCategories: topCategories,
		MonthlyTrend:  monthlyTrend,
	}, nil
}

// GetTransactionSummary computes an aggregated summary of customer transactions.
// Balance reflects the real account balance, not just sum of transactions.
func (s *BankingService) GetTransactionSummary(ctx context.Context, customerID string) (*domain.TransactionSummary, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.GetTransactionSummary")
	defer span.End()

	summary, err := s.store.GetTransactionSummary(ctx, customerID)
	if err != nil {
		return nil, err
	}

	// Override balance with real account balance
	account, acctErr := s.store.GetPrimaryAccount(ctx, customerID)
	if acctErr == nil && account != nil {
		summary.Balance = account.Balance
	}

	return summary, nil
}

// ============================================================
// Pix Key Registration
// ============================================================

// RegisterPixKey creates a new Pix key for the given customer.
func (s *BankingService) RegisterPixKey(ctx context.Context, req *domain.PixKeyRegisterRequest) (*domain.PixKeyRegisterResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.RegisterPixKey")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", req.CustomerID))

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}

	validTypes := map[string]bool{"cnpj": true, "email": true, "phone": true, "random": true}
	if !validTypes[req.KeyType] {
		return nil, &domain.ErrValidation{Field: "keyType", Message: "deve ser cnpj, email, phone ou random"}
	}

	// Get primary account for account_id
	account, err := s.store.GetPrimaryAccount(ctx, req.CustomerID)
	if err != nil {
		return nil, err
	}

	keyValue := req.KeyValue
	if req.KeyType == "random" {
		keyValue = uuid.New().String()
	} else if keyValue == "" {
		return nil, &domain.ErrValidation{Field: "keyValue", Message: "required for non-random key type"}
	}

	key := &domain.PixKey{
		ID:         uuid.New().String(),
		AccountID:  account.ID,
		CustomerID: req.CustomerID,
		KeyType:    req.KeyType,
		KeyValue:   keyValue,
		Status:     "active",
		CreatedAt:  time.Now(),
	}

	created, err := s.store.CreatePixKey(ctx, key)
	if err != nil {
		s.logger.Error("failed to register pix key",
			zap.String("customer_id", req.CustomerID),
			zap.String("key_type", req.KeyType),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Info("pix key registered",
		zap.String("customer_id", req.CustomerID),
		zap.String("key_type", req.KeyType),
		zap.String("key_id", created.ID),
	)

	return &domain.PixKeyRegisterResponse{
		KeyID:     created.ID,
		KeyType:   created.KeyType,
		KeyValue:  created.KeyValue,
		Key:       created.KeyValue,
		Status:    created.Status,
		CreatedAt: created.CreatedAt.Format(time.RFC3339),
	}, nil
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

// ============================================================
// Dev Tools
// ============================================================

// DevAddBalance adds the given amount to the customer's primary account balance.
func (s *BankingService) DevAddBalance(ctx context.Context, req *domain.DevAddBalanceRequest) (*domain.DevAddBalanceResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevAddBalance")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.Amount == 0 {
		return nil, &domain.ErrValidation{Field: "amount", Message: "não pode ser zero"}
	}

	acct, err := s.store.UpdateAccountBalance(ctx, req.CustomerID, req.Amount)
	if err != nil {
		return nil, err
	}

	// Record the transaction for extrato/fatura
	now := time.Now()
	txType := "transfer_in"
	txDesc := fmt.Sprintf("DevTools — Crédito de saldo R$ %.2f", req.Amount)
	if req.Amount < 0 {
		txType = "transfer_out"
		txDesc = fmt.Sprintf("DevTools — Débito de saldo R$ %.2f", -req.Amount)
	}
	tx := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": req.CustomerID,
		"date":        now.Format(time.RFC3339),
		"description": txDesc,
		"amount":      req.Amount,
		"type":        txType,
		"category":    "devtools",
	}
	if txErr := s.store.InsertTransaction(ctx, tx); txErr != nil {
		s.logger.Error("DEV: failed to record balance transaction",
			zap.String("customer_id", req.CustomerID),
			zap.Error(txErr),
		)
		// Don't fail the whole operation — balance was already updated
	}

	s.logger.Info("DEV: balance adjusted",
		zap.String("customer_id", req.CustomerID),
		zap.Float64("amount", req.Amount),
		zap.Float64("new_balance", acct.Balance),
	)

	msg := fmt.Sprintf("R$ %.2f adicionados ao saldo", req.Amount)
	if req.Amount < 0 {
		msg = fmt.Sprintf("R$ %.2f debitados do saldo", -req.Amount)
	}
	return &domain.DevAddBalanceResponse{
		Success:    true,
		NewBalance: acct.Balance,
		Message:    msg,
	}, nil
}

// DevSetCreditLimit sets the credit limit of the customer's first credit card.
func (s *BankingService) DevSetCreditLimit(ctx context.Context, req *domain.DevSetCreditLimitRequest) (*domain.DevSetCreditLimitResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevSetCreditLimit")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.CreditLimit <= 0 {
		return nil, &domain.ErrValidation{Field: "creditLimit", Message: "deve ser positivo"}
	}

	err := s.store.UpdateCreditCardLimit(ctx, req.CustomerID, req.CreditLimit)
	if err != nil {
		return nil, err
	}

	// Record the transaction for extrato/fatura
	now := time.Now()
	tx := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": req.CustomerID,
		"date":        now.Format(time.RFC3339),
		"description": fmt.Sprintf("DevTools — Limite de crédito ajustado para R$ %.2f", req.CreditLimit),
		"amount":      0,
		"type":        "credit",
		"category":    "devtools",
	}
	if txErr := s.store.InsertTransaction(ctx, tx); txErr != nil {
		s.logger.Error("DEV: failed to record credit limit transaction",
			zap.String("customer_id", req.CustomerID),
			zap.Error(txErr),
		)
	}

	s.logger.Info("DEV: credit limit updated",
		zap.String("customer_id", req.CustomerID),
		zap.Float64("new_limit", req.CreditLimit),
	)

	return &domain.DevSetCreditLimitResponse{
		Success:  true,
		NewLimit: req.CreditLimit,
		Message:  fmt.Sprintf("Limite de crédito atualizado para R$ %.2f", req.CreditLimit),
	}, nil
}

// DevGenerateTransactions generates random transactions for testing.
func (s *BankingService) DevGenerateTransactions(ctx context.Context, req *domain.DevGenerateTransactionsRequest) (*domain.DevGenerateTransactionsResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevGenerateTransactions")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.Count <= 0 || req.Count > 100 {
		return nil, &domain.ErrValidation{Field: "count", Message: "deve ser entre 1 e 100"}
	}

	// Default months = 1, max 12
	months := req.Months
	// If period is set, it overrides months
	switch req.Period {
	case "current-month":
		months = 1
	case "last-12-months":
		months = 12
	}
	if months <= 0 {
		months = 1
	}
	if months > 12 {
		months = 12
	}
	daysSpan := months * 30 // approximate days to spread transactions across

	txTypes := []struct {
		Type     string
		IsDebit  bool
		Descs    []string
		Category string
	}{
		{"pix_sent", true, []string{"Pix enviado - Maria Silva", "Pix enviado - João LTDA", "Pix enviado - Ana Costa"}, "pix"},
		{"pix_received", false, []string{"Pix recebido - Tech Corp", "Pix recebido - Vendas Online", "Pix recebido - Cliente ABC"}, "recebimento"},
		{"debit_purchase", true, []string{"Supermercado Extra", "Posto Shell", "Farmácia São Paulo", "Restaurante Sabor"}, "compras"},
		{"credit_purchase", true, []string{"Amazon AWS", "Google Cloud", "Material Escritório", "Uber Business"}, "tecnologia"},
		{"transfer_in", false, []string{"TED recebida - Fornecedor A", "DOC recebido - Partner B", "Transferência recebida - Cliente"}, "recebimento"},
		{"transfer_out", true, []string{"TED enviada - Aluguel", "TED enviada - Fornecedor", "Transferência - Pagamento"}, "despesas"},
		{"bill_payment", true, []string{"Conta de luz", "Conta de telefone", "Internet Fibra", "IPTU"}, "contas"},
		{"credit", false, []string{"Crédito recebido", "Estorno - Compra duplicada", "Bonificação empresarial"}, "credito"},
		{"debit", true, []string{"Débito automático", "Tarifa bancária", "Cobrança serviço"}, "debito"},
	}

	generated := 0
	netImpact := 0.0
	totalIncome := 0.0
	totalExpenses := 0.0
	now := time.Now()

	for i := 0; i < req.Count; i++ {
		txInfo := txTypes[rand.Intn(len(txTypes))]
		desc := txInfo.Descs[rand.Intn(len(txInfo.Descs))]
		amount := float64(rand.Intn(490000)+1000) / 100.0 // R$ 10.00 to R$ 5000.00
		daysAgo := rand.Intn(daysSpan)
		txDate := now.AddDate(0, 0, -daysAgo)

		if txInfo.IsDebit {
			amount = -amount
		}

		tx := map[string]any{
			"id":          uuid.New().String(),
			"customer_id": req.CustomerID,
			"date":        txDate.Format(time.RFC3339),
			"description": desc,
			"amount":      amount,
			"type":        txInfo.Type,
			"category":    txInfo.Category,
		}

		if err := s.store.InsertTransaction(ctx, tx); err != nil {
			s.logger.Warn("DEV: failed to insert transaction", zap.Int("index", i), zap.Error(err))
			continue
		}
		generated++
		netImpact += amount // amount is already negative for debits
		if amount > 0 {
			totalIncome += amount
		} else {
			totalExpenses += -amount // store as positive value
		}
	}

	// Update the account balance to reflect the net impact of generated transactions
	if netImpact != 0 {
		if _, balErr := s.store.UpdateAccountBalance(ctx, req.CustomerID, netImpact); balErr != nil {
			s.logger.Error("DEV: failed to update balance after generating transactions",
				zap.String("customer_id", req.CustomerID),
				zap.Float64("net_impact", netImpact),
				zap.Error(balErr),
			)
		} else {
			s.logger.Info("DEV: balance adjusted after transaction generation",
				zap.Float64("net_impact", netImpact),
			)
		}
	}

	s.logger.Info("DEV: transactions generated",
		zap.String("customer_id", req.CustomerID),
		zap.Int("generated", generated),
	)

	return &domain.DevGenerateTransactionsResponse{
		Success:   true,
		Generated: generated,
		Income:    totalIncome,
		Expenses:  totalExpenses,
		NetImpact: netImpact,
		Message:   fmt.Sprintf("%d transações geradas com sucesso", generated),
	}, nil
}

// DevAddCardPurchase simulates credit card purchases for testing.
func (s *BankingService) DevAddCardPurchase(ctx context.Context, req *domain.DevAddCardPurchaseRequest) (*domain.DevAddCardPurchaseResponse, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.DevAddCardPurchase")
	defer span.End()

	if req.CustomerID == "" {
		return nil, &domain.ErrValidation{Field: "customerId", Message: "required"}
	}
	if req.CardID == "" {
		return nil, &domain.ErrValidation{Field: "cardId", Message: "required"}
	}
	if req.Amount <= 0 {
		return nil, &domain.ErrValidation{Field: "amount", Message: "deve ser positivo"}
	}
	if req.Mode != "today" && req.Mode != "random" {
		return nil, &domain.ErrValidation{Field: "mode", Message: "deve ser 'today' ou 'random'"}
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Mode == "today" {
		req.Count = 1
	}
	if req.Count > 50 {
		return nil, &domain.ErrValidation{Field: "count", Message: "máximo 50"}
	}

	// Verify card exists; auto-activate if pending
	card, err := s.store.GetCreditCard(ctx, req.CustomerID, req.CardID)
	if err != nil {
		return nil, err
	}
	if card.Status == "pending_activation" {
		if activateErr := s.store.UpdateCreditCardStatus(ctx, req.CardID, "active"); activateErr != nil {
			s.logger.Warn("DEV: failed to auto-activate card", zap.String("card_id", req.CardID), zap.Error(activateErr))
		} else {
			card.Status = "active"
			s.logger.Info("DEV: auto-activated pending card", zap.String("card_id", req.CardID))
		}
	}
	if card.Status != "active" {
		return nil, &domain.ErrValidation{Field: "cardId", Message: "cartão não está ativo"}
	}

	merchants := []struct {
		Name     string
		Category string
	}{
		{"Restaurante Sabor & Arte", "food"},
		{"Posto Shell BR-101", "fuel"},
		{"Amazon AWS", "technology"},
		{"Uber Business", "transport"},
		{"Netflix Assinatura", "subscription"},
		{"Google Cloud Platform", "technology"},
		{"iFood Corporativo", "food"},
		{"Kalunga Papelaria", "office_supplies"},
		{"99 Táxi Corporativo", "transport"},
		{"Adobe Creative Cloud", "subscription"},
		{"Hotel Ibis Business", "travel"},
		{"Seguro Porto PJ", "insurance"},
		{"Copel Energia", "utilities"},
		{"Google Ads", "marketing"},
		{"Contabilidade Express", "professional_services"},
		{"DAS Simples Nacional", "tax"},
		{"Limpeza & Manutenção", "maintenance"},
	}

	now := time.Now()
	generated := 0
	var totalAmount float64

	// Determine target month boundaries
	var monthStart, monthEnd time.Time
	if req.TargetMonth != "" {
		// Parse "YYYY-MM"
		parsed, parseErr := time.Parse("2006-01", req.TargetMonth)
		if parseErr != nil {
			return nil, &domain.ErrValidation{Field: "targetMonth", Message: "formato deve ser YYYY-MM"}
		}
		monthStart = parsed
		monthEnd = parsed.AddDate(0, 1, 0).Add(-time.Second) // last second of that month
	} else {
		// Default: current month
		monthStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		monthEnd = now
	}

	for i := 0; i < req.Count; i++ {
		m := merchants[rand.Intn(len(merchants))]

		var txDate time.Time
		if req.Mode == "today" && req.TargetMonth == "" {
			txDate = now
		} else {
			// Random date within the target month range
			dayRange := int(monthEnd.Sub(monthStart).Hours()/24) + 1
			if dayRange < 1 {
				dayRange = 1
			}
			randomDay := rand.Intn(dayRange)
			txDate = monthStart.AddDate(0, 0, randomDay)
			// Add random hour
			txDate = txDate.Add(time.Duration(rand.Intn(14)+8) * time.Hour)
			txDate = txDate.Add(time.Duration(rand.Intn(60)) * time.Minute)
		}

		tx := map[string]any{
			"id":                  uuid.New().String(),
			"card_id":             req.CardID,
			"customer_id":         req.CustomerID,
			"transaction_date":    txDate.Format(time.RFC3339),
			"amount":              req.Amount,
			"merchant_name":       m.Name,
			"category":            m.Category,
			"description":         fmt.Sprintf("Compra - %s", m.Name),
			"installments":        1,
			"current_installment": 1,
			"transaction_type":    "purchase",
			"status":              "confirmed",
		}

		if txErr := s.store.InsertCreditCardTransaction(ctx, tx); txErr != nil {
			s.logger.Warn("DEV: failed to insert card purchase", zap.Int("index", i), zap.Error(txErr))
			continue
		}
		generated++
		totalAmount += req.Amount
	}

	// Update card used_limit and available_limit
	if totalAmount > 0 {
		newUsed := card.UsedLimit + totalAmount
		newAvailable := card.CreditLimit - newUsed
		if newAvailable < 0 {
			newAvailable = 0
		}
		if err := s.store.UpdateCreditCardUsedLimit(ctx, req.CardID, newUsed, newAvailable); err != nil {
			s.logger.Error("DEV: failed to update card limits",
				zap.String("card_id", req.CardID),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("DEV: card purchases generated",
		zap.String("customer_id", req.CustomerID),
		zap.String("card_id", req.CardID),
		zap.Int("generated", generated),
		zap.Float64("total_amount", totalAmount),
	)

	return &domain.DevAddCardPurchaseResponse{
		Success:     true,
		Generated:   generated,
		TotalAmount: totalAmount,
		Message:     fmt.Sprintf("%d compras adicionadas ao cartão •••• %s", generated, card.CardNumberLast4),
	}, nil
}
