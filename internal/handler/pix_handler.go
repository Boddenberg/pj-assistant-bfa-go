package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ============================================================
// 5. PIX Handlers
// ============================================================

func pixKeyLookupHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/pix/keys/lookup")
		defer span.End()

		keyValue := r.URL.Query().Get("key")
		keyType := r.URL.Query().Get("keyType")
		if keyType == "" {
			keyType = r.URL.Query().Get("type") // alias: ?type=email
		}

		pixKey, err := bankSvc.LookupPixKey(ctx, keyType, keyValue)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		// Resolve the customer profile + account for the recipient display
		recipientName, recipientDoc, recipientBank, recipientBranch, recipientAcct, lookupErr := bankSvc.GetCustomerLookupData(ctx, pixKey.CustomerID)
		if lookupErr != nil {
			logger.Warn("could not resolve recipient data", zap.String("customer_id", pixKey.CustomerID), zap.Error(lookupErr))
			recipientName = "Destinatário"
			recipientBank = "Itaú Unibanco"
		}

		resp := domain.PixKeyLookupResponse{
			KeyType: pixKey.KeyType,
			Recipient: &domain.PixRecipient{
				Name:     recipientName,
				Document: recipientDoc,
				Bank:     recipientBank,
				Branch:   recipientBranch,
				Account:  recipientAcct,
				PixKey: &domain.PixKeyInfo{
					Type:  pixKey.KeyType,
					Value: pixKey.KeyValue,
				},
			},
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func pixTransferHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/pix/transfer")
		defer span.End()

		var apiReq struct {
			CustomerID             string  `json:"customerId"`
			RecipientKey           string  `json:"recipientKey"`
			RecipientKeyType       string  `json:"recipientKeyType"`
			Amount                 float64 `json:"amount"`
			Description            string  `json:"description,omitempty"`
			FundedBy               string  `json:"fundedBy,omitempty"`
			CreditCardID           string  `json:"creditCardId,omitempty"`
			CreditCardInstallments int     `json:"installments,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		account, err := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		fundedBy := apiReq.FundedBy
		if fundedBy == "" {
			fundedBy = "balance"
		}

		req := &domain.PixTransferRequest{
			IdempotencyKey:         uuid.New().String(),
			SourceAccountID:        account.ID,
			DestinationKeyType:     apiReq.RecipientKeyType,
			DestinationKeyValue:    apiReq.RecipientKey,
			Amount:                 apiReq.Amount,
			Description:            apiReq.Description,
			FundedBy:               fundedBy,
			CreditCardID:           apiReq.CreditCardID,
			CreditCardInstallments: apiReq.CreditCardInstallments,
		}

		transfer, err := bankSvc.CreatePixTransfer(ctx, apiReq.CustomerID, req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		// Fetch updated balance for response
		var newBalance float64
		if updatedAcct, balErr := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID); balErr == nil {
			newBalance = updatedAcct.AvailableBalance
		}

		resp := domain.PixTransferResponse{
			TransactionID: transfer.ID,
			Status:        transfer.Status,
			Amount:        transfer.Amount,
			NewBalance:    newBalance,
			Timestamp:     transfer.CreatedAt.Format(time.RFC3339),
			E2EID:         transfer.EndToEndID,
			ReceiptID:     transfer.ReceiptID,
			Recipient: &domain.PixRecipient{
				Name:     transfer.DestinationName,
				Document: transfer.DestinationDocument,
				Bank:     "Itaú Unibanco",
				PixKey: &domain.PixKeyInfo{
					Type:  transfer.DestinationKeyType,
					Value: transfer.DestinationKeyValue,
				},
			},
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func pixScheduleHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/pix/schedule")
		defer span.End()

		var apiReq domain.PixScheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		account, err := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		schedType := "once"
		recEndDate := ""
		if apiReq.Recurrence != nil {
			schedType = apiReq.Recurrence.Type
			recEndDate = apiReq.Recurrence.EndDate
		}

		req := &domain.ScheduledTransferRequest{
			IdempotencyKey:    uuid.New().String(),
			SourceAccountID:   account.ID,
			TransferType:      "pix",
			Amount:            apiReq.Amount,
			Description:       apiReq.Description,
			ScheduleType:      schedType,
			ScheduledDate:     apiReq.ScheduledDate,
			RecurrenceEndDate: recEndDate,
		}

		transfer, err := bankSvc.CreateScheduledTransfer(ctx, apiReq.CustomerID, req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := domain.PixScheduleResponse{
			ScheduleID:    transfer.ID,
			Status:        transfer.Status,
			Amount:        transfer.Amount,
			ScheduledDate: transfer.ScheduledDate,
			Recipient: &domain.PixRecipient{
				Name: transfer.DestinationName,
			},
			Recurrence: apiReq.Recurrence,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func pixScheduleDeleteHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "DELETE /v1/pix/schedule/{scheduleId}")
		defer span.End()

		scheduleID := chi.URLParam(r, "scheduleId")
		if err := bankSvc.CancelScheduledTransferByID(ctx, scheduleID); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func pixScheduledListHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/pix/scheduled")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")

		transfers, err := bankSvc.ListScheduledTransfers(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := make([]domain.PixScheduleResponse, 0, len(transfers))
		for _, t := range transfers {
			item := domain.PixScheduleResponse{
				ScheduleID:    t.ID,
				Status:        t.Status,
				Amount:        t.Amount,
				ScheduledDate: t.ScheduledDate,
				Recipient: &domain.PixRecipient{
					Name:     t.DestinationName,
					Document: t.DestinationDocument,
					Bank:     t.DestinationBankCode,
					Branch:   t.DestinationBranch,
					Account:  t.DestinationAccount,
				},
			}
			if t.ScheduleType != "once" {
				item.Recurrence = &domain.ScheduleRecurrence{Type: t.ScheduleType}
			}
			resp = append(resp, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{"schedules": resp})
	}
}

// pixScheduledListByParamHandler is an alias using GET /v1/pix/scheduled/{customerId}
func pixScheduledListByParamHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return pixScheduledListHandler(bankSvc, logger)
}

func pixCreditCardHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/pix/credit-card")
		defer span.End()

		var apiReq domain.PixCreditCardRequest
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if apiReq.Installments <= 0 {
			apiReq.Installments = 1
		}
		if apiReq.Installments > 12 {
			writeError(w, http.StatusBadRequest, "installments must be between 1 and 12")
			return
		}
		if apiReq.Amount <= 0 {
			writeError(w, http.StatusBadRequest, "amount must be positive")
			return
		}
		if apiReq.CreditCardID == "" {
			writeError(w, http.StatusBadRequest, "creditCardId is required")
			return
		}
		if apiReq.RecipientKey == "" {
			writeError(w, http.StatusBadRequest, "recipientKey is required")
			return
		}

		account, err := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		feeRate := 0.02
		totalWithFees := apiReq.Amount * (1 + feeRate*float64(apiReq.Installments-1))
		installmentValue := totalWithFees / float64(apiReq.Installments)

		req := &domain.PixTransferRequest{
			IdempotencyKey:         uuid.New().String(),
			SourceAccountID:        account.ID,
			DestinationKeyType:     apiReq.RecipientKeyType,
			DestinationKeyValue:    apiReq.RecipientKey,
			Amount:                 apiReq.Amount,
			Description:            apiReq.Description,
			FundedBy:               "credit_card",
			CreditCardID:           apiReq.CreditCardID,
			CreditCardInstallments: apiReq.Installments,
			FeeRate:                feeRate,
			TotalWithFees:          totalWithFees,
		}

		transfer, err := bankSvc.CreatePixTransfer(ctx, apiReq.CustomerID, req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		feeAmount := totalWithFees - apiReq.Amount

		resp := domain.PixCreditCardResponse{
			TransactionID:    transfer.ID,
			Status:           transfer.Status,
			Amount:           apiReq.Amount,
			OriginalAmount:   apiReq.Amount,
			FeeAmount:        feeAmount,
			TotalWithFees:    totalWithFees,
			Installments:     apiReq.Installments,
			InstallmentValue: installmentValue,
			Recipient: &domain.PixRecipient{
				Name: transfer.DestinationName,
				PixKey: &domain.PixKeyInfo{
					Type:  transfer.DestinationKeyType,
					Value: transfer.DestinationKeyValue,
				},
			},
			Timestamp: transfer.CreatedAt.Format(time.RFC3339),
			ReceiptID: transfer.ReceiptID,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

// ============================================================
// 5b. PIX Receipts (Comprovantes)
// ============================================================

func formatReceiptResponse(r *domain.PixReceipt) *domain.PixReceiptResponse {
	return &domain.PixReceiptResponse{
		ID:             r.ID,
		TransferID:     r.TransferID,
		Direction:      r.Direction,
		Amount:         r.Amount,
		OriginalAmount: r.OriginalAmount,
		FeeAmount:      r.FeeAmount,
		TotalAmount:    r.TotalAmount,
		Description:    r.Description,
		E2EID:          r.EndToEndID,
		FundedBy:       r.FundedBy,
		Installments:   r.Installments,
		Sender: &domain.PixReceiptParty{
			Name:     r.SenderName,
			Document: r.SenderDocument,
			Bank:     r.SenderBank,
			Branch:   r.SenderBranch,
			Account:  r.SenderAccount,
		},
		Recipient: &domain.PixReceiptParty{
			Name:     r.RecipientName,
			Document: r.RecipientDocument,
			Bank:     r.RecipientBank,
			Branch:   r.RecipientBranch,
			Account:  r.RecipientAccount,
		},
		PixKey: &domain.PixKeyInfo{
			Type:  r.RecipientKeyType,
			Value: r.RecipientKeyValue,
		},
		Status:     r.Status,
		ExecutedAt: r.ExecutedAt,
		CreatedAt:  r.CreatedAt,
	}
}

func getPixReceiptHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/pix/receipts/{receiptId}")
		defer span.End()

		receiptID := chi.URLParam(r, "receiptId")
		if receiptID == "" {
			writeError(w, http.StatusBadRequest, "receiptId is required")
			return
		}

		receipt, err := bankSvc.GetPixReceipt(ctx, receiptID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, formatReceiptResponse(receipt))
	}
}

func getPixReceiptByTransferHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/pix/transfers/{transferId}/receipt")
		defer span.End()

		transferID := chi.URLParam(r, "transferId")
		if transferID == "" {
			writeError(w, http.StatusBadRequest, "transferId is required")
			return
		}

		receipt, err := bankSvc.GetPixReceiptByTransferID(ctx, transferID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, formatReceiptResponse(receipt))
	}
}

func listPixReceiptsHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/pix/receipts")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		if customerID == "" {
			writeError(w, http.StatusBadRequest, "customerId is required")
			return
		}

		receipts, err := bankSvc.ListPixReceipts(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		result := make([]*domain.PixReceiptResponse, 0, len(receipts))
		for i := range receipts {
			result = append(result, formatReceiptResponse(&receipts[i]))
		}

		writeJSON(w, http.StatusOK, map[string]any{"receipts": result})
	}
}

// ============================================================
// Pix Key Registration Handler
// ============================================================

func pixKeyRegisterHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/pix/keys/register")
		defer span.End()

		var req domain.PixKeyRegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.RegisterPixKey(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func pixKeyDeleteByValueHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "DELETE /v1/pix/keys")
		defer span.End()

		var req struct {
			CustomerID string `json:"customerId"`
			KeyType    string `json:"keyType"`
			KeyValue   string `json:"keyValue"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.CustomerID == "" || req.KeyType == "" || req.KeyValue == "" {
			writeError(w, http.StatusBadRequest, "customerId, keyType and keyValue are required")
			return
		}

		if err := svc.DeletePixKeyByValue(ctx, req.CustomerID, req.KeyType, req.KeyValue); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Chave Pix removida com sucesso."})
	}
}

func listPixKeysHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /pix/keys")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		keys, err := svc.ListPixKeys(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		if keys == nil {
			keys = []domain.PixKey{}
		}
		type pixKeyDisplay struct {
			domain.PixKey
			FormattedValue string `json:"formatted_value"`
		}
		result := make([]pixKeyDisplay, len(keys))
		for i, k := range keys {
			result[i] = pixKeyDisplay{
				PixKey:         k,
				FormattedValue: formatKeyValue(k.KeyType, k.KeyValue),
			}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func deletePixKeyHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "DELETE /pix/keys/{keyId}")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		keyID := chi.URLParam(r, "keyId")
		if err := svc.DeletePixKey(ctx, customerID, keyID); err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Chave Pix excluída com sucesso"})
	}
}

func creditLimitHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/credit-limit")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		limit, err := svc.GetCreditLimit(ctx, customerID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"creditLimit": 0})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"creditLimit": limit})
	}
}
