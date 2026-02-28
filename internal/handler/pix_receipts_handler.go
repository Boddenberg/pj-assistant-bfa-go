package handler

import (
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ============================================================
// PIX Receipts (Comprovantes)
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
