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
// PIX Transfer — create, list, get, cancel
// ============================================================

func (s *BankingService) CreatePixTransfer(ctx context.Context, customerID string, req *domain.PixTransferRequest) (*domain.PixTransfer, error) {
	ctx, span := bankTracer.Start(ctx, "BankingService.CreatePixTransfer")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID), attribute.Float64("amount", req.Amount))

	start := time.Now()
	defer func() { s.metrics.RecordRequestDuration("pix_transfer", time.Since(start)) }()

	// ── Validate inputs ──
	if err := validatePixTransferRequest(req); err != nil {
		return nil, err
	}

	// Check account exists and belongs to customer
	account, err := s.store.GetAccount(ctx, customerID, req.SourceAccountID)
	if err != nil {
		return nil, err
	}

	// Block self-transfer
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

	// ── Check limits ──
	if err := s.checkPixLimits(ctx, customerID, req); err != nil {
		return nil, err
	}

	// ── Check funding source ──
	if err := s.checkPixFunding(ctx, customerID, account, req); err != nil {
		return nil, err
	}

	// ── Resolve destination info ──
	var destCustomerID string
	if destKey != nil {
		destCustomerID = destKey.CustomerID
		destName, destDoc, _, _, _, lookupErr := s.store.GetCustomerLookupData(ctx, destKey.CustomerID)
		if lookupErr == nil {
			req.DestinationName = destName
			req.DestinationDocument = destDoc
		}
	}

	// ── Resolve sender & destination lookup data for receipts ──
	senderName, senderDoc, senderBank, senderBranch, senderAcct := s.resolveSenderData(ctx, customerID)
	destBank, destBranch, destAcct := s.resolveDestData(ctx, destCustomerID)

	// ── Persist transfer ──
	transfer, err := s.store.CreatePixTransfer(ctx, customerID, req)
	if err != nil {
		s.logger.Error("failed to create PIX transfer", zap.Error(err))
		return nil, err
	}

	now := time.Now()

	// ── 1. Debit sender ──
	descSent := formatPixDescription("Pix enviado", transfer.DestinationName, transfer.DestinationKeyValue)
	s.debitSender(ctx, customerID, req, descSent, now)

	// ── 2. Credit destination ──
	s.creditDestination(ctx, destCustomerID, senderName, req.Amount, now)

	// ── 3. Mark transfer as completed ──
	if updErr := s.store.UpdatePixTransferStatus(ctx, transfer.ID, "completed"); updErr != nil {
		s.logger.Error("failed to update pix transfer status to completed",
			zap.String("transfer_id", transfer.ID), zap.Error(updErr))
	} else {
		transfer.Status = "completed"
	}

	// ── 4. Save receipts ──
	transfer.ReceiptID = s.savePixReceipts(ctx, transfer, customerID, destCustomerID, req, senderName, senderDoc, senderBank, senderBranch, senderAcct, destBank, destBranch, destAcct, now)

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
// Private helpers — keep CreatePixTransfer readable
// ============================================================

func validatePixTransferRequest(req *domain.PixTransferRequest) error {
	if req.Amount <= 0 {
		return &domain.ErrValidation{Field: "amount", Message: "must be positive"}
	}
	if req.DestinationKeyValue == "" {
		return &domain.ErrValidation{Field: "destination_key_value", Message: "required"}
	}
	if req.IdempotencyKey == "" {
		return &domain.ErrValidation{Field: "idempotency_key", Message: "required"}
	}
	if req.SourceAccountID == "" {
		return &domain.ErrValidation{Field: "source_account_id", Message: "required"}
	}
	if req.FundedBy == "" {
		req.FundedBy = "balance"
	}
	return nil
}

func (s *BankingService) checkPixLimits(ctx context.Context, customerID string, req *domain.PixTransferRequest) error {
	limit, err := s.store.GetTransactionLimit(ctx, customerID, "pix")
	if err == nil && limit != nil {
		if req.Amount > limit.SingleLimit {
			return &domain.ErrLimitExceeded{LimitType: "single_pix", Limit: limit.SingleLimit, Current: req.Amount}
		}
		if limit.DailyUsed+req.Amount > limit.DailyLimit {
			return &domain.ErrLimitExceeded{LimitType: "daily_pix", Limit: limit.DailyLimit, Current: limit.DailyUsed + req.Amount}
		}
	}
	return nil
}

func (s *BankingService) checkPixFunding(ctx context.Context, customerID string, account *domain.Account, req *domain.PixTransferRequest) error {
	if req.FundedBy == "balance" && account.AvailableBalance < req.Amount {
		return &domain.ErrInsufficientFunds{Available: account.AvailableBalance, Required: req.Amount}
	}

	if req.FundedBy == "credit_card" {
		if req.CreditCardID == "" {
			return &domain.ErrValidation{Field: "credit_card_id", Message: "required when funded_by is credit_card"}
		}
		card, err := s.store.GetCreditCard(ctx, customerID, req.CreditCardID)
		if err != nil {
			return err
		}
		if !card.PixCreditEnabled {
			return &domain.ErrValidation{Field: "credit_card_id", Message: "PIX via credit card not enabled for this card"}
		}
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
		if card.PixCreditUsed+req.TotalWithFees > card.PixCreditLimit {
			return &domain.ErrLimitExceeded{LimitType: "pix_credit", Limit: card.PixCreditLimit, Current: card.PixCreditUsed + req.TotalWithFees}
		}
	}
	return nil
}

func (s *BankingService) resolveSenderData(ctx context.Context, customerID string) (name, doc, bank, branch, acct string) {
	name, _ = s.store.GetCustomerName(ctx, customerID)
	if name == "" || name == "Destinatário" {
		name = "Remetente"
	}
	if sName, sDoc, sBank, sBranch, sAcct, sErr := s.store.GetCustomerLookupData(ctx, customerID); sErr == nil {
		if name == "Remetente" {
			name = sName
		}
		doc = sDoc
		bank = sBank
		branch = sBranch
		acct = sAcct
	}
	return
}

func (s *BankingService) resolveDestData(ctx context.Context, destCustomerID string) (bank, branch, acct string) {
	if destCustomerID != "" {
		if _, _, dBank, dBranch, dAcct, dErr := s.store.GetCustomerLookupData(ctx, destCustomerID); dErr == nil {
			bank = dBank
			branch = dBranch
			acct = dAcct
		}
	}
	return
}

func formatPixDescription(prefix, destName, destKeyValue string) string {
	if destName != "" {
		return fmt.Sprintf("%s - %s", prefix, destName)
	}
	return fmt.Sprintf("%s - %s", prefix, destKeyValue)
}

func (s *BankingService) debitSender(ctx context.Context, customerID string, req *domain.PixTransferRequest, descSent string, now time.Time) {
	if req.FundedBy == "credit_card" {
		s.debitSenderCreditCard(ctx, customerID, req, descSent, now)
	} else {
		s.debitSenderBalance(ctx, customerID, req.Amount, descSent, now)
	}
}

func (s *BankingService) debitSenderCreditCard(ctx context.Context, customerID string, req *domain.PixTransferRequest, descSent string, now time.Time) {
	card, _ := s.store.GetCreditCard(ctx, customerID, req.CreditCardID)
	if card != nil {
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
		newPixUsed := card.PixCreditUsed + debitAmount
		if pxErr := s.store.UpdateCreditCardPixCreditUsed(ctx, card.ID, newPixUsed); pxErr != nil {
			s.logger.Error("failed to update pix_credit_used after pix credit",
				zap.String("card_id", card.ID), zap.Error(pxErr))
		}
	}

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
}

func (s *BankingService) debitSenderBalance(ctx context.Context, customerID string, amount float64, descSent string, now time.Time) {
	if _, balErr := s.store.UpdateAccountBalance(ctx, customerID, -amount); balErr != nil {
		s.logger.Error("failed to debit sender balance after pix transfer",
			zap.String("customer_id", customerID), zap.Error(balErr))
	}

	txSent := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": customerID,
		"date":        now.Format(time.RFC3339),
		"description": descSent,
		"amount":      -amount,
		"type":        "pix_sent",
		"category":    "pix",
	}
	if txErr := s.store.InsertTransaction(ctx, txSent); txErr != nil {
		s.logger.Error("failed to record sender pix transaction",
			zap.String("customer_id", customerID), zap.Error(txErr))
	}
}

func (s *BankingService) creditDestination(ctx context.Context, destCustomerID, senderName string, amount float64, now time.Time) {
	if destCustomerID == "" {
		return
	}

	if _, balErr := s.store.UpdateAccountBalance(ctx, destCustomerID, amount); balErr != nil {
		s.logger.Error("failed to credit destination balance after pix transfer",
			zap.String("dest_customer_id", destCustomerID), zap.Error(balErr))
	} else {
		s.logger.Info("PIX destination credited",
			zap.String("dest_customer_id", destCustomerID),
			zap.Float64("amount", amount))
	}

	txReceived := map[string]any{
		"id":          uuid.New().String(),
		"customer_id": destCustomerID,
		"date":        now.Format(time.RFC3339),
		"description": fmt.Sprintf("Pix recebido - %s", senderName),
		"amount":      amount,
		"type":        "pix_received",
		"category":    "recebimento",
	}
	if txErr := s.store.InsertTransaction(ctx, txReceived); txErr != nil {
		s.logger.Error("failed to record destination pix_received transaction",
			zap.String("dest_customer_id", destCustomerID), zap.Error(txErr))
	}
}

func (s *BankingService) savePixReceipts(ctx context.Context, transfer *domain.PixTransfer, customerID, destCustomerID string, req *domain.PixTransferRequest, senderName, senderDoc, senderBank, senderBranch, senderAcct, destBank, destBranch, destAcct string, now time.Time) string {
	nowStr := now.Format(time.RFC3339)
	installments := req.CreditCardInstallments
	if installments <= 0 {
		installments = 1
	}
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

	var receiptID string
	if savedReceipt, rcptErr := s.store.SavePixReceipt(ctx, receiptSent); rcptErr != nil {
		s.logger.Error("failed to save pix receipt for sender",
			zap.String("transfer_id", transfer.ID), zap.Error(rcptErr))
	} else {
		receiptID = savedReceipt.ID
	}

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

	return receiptID
}
