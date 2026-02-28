package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ============================================================
// PIX Transfer — single transfer + credit-card transfer
// ============================================================

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
