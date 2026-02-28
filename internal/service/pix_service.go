package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// ============================================================
// PIX Keys
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

// ============================================================
// PIX Transfer
// ============================================================

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
			"category":            "other",
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

		// Also record in customer_transactions so it appears in extrato and despesas
		txSentCC := map[string]any{
			"id":          uuid.New().String(),
			"customer_id": customerID,
			"date":        now.Format(time.RFC3339),
			"description": descSent,
			"amount":      -faturaAmount,
			"type":        "pix_sent",
			"category":    "pix",
		}
		if txErr := s.store.InsertTransaction(ctx, txSentCC); txErr != nil {
			s.logger.Error("failed to record pix credit card in customer_transactions",
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
