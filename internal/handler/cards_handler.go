package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ============================================================
// 7. Cartão de Crédito
// ============================================================

func listCardsHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/cards")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")

		cards, err := bankSvc.ListCreditCards(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := make([]domain.CreditCardAPIResponse, 0, len(cards))
		for _, c := range cards {
			isVirtual := c.CardType == "virtual"
			resp = append(resp, domain.CreditCardAPIResponse{
				ID:             c.ID,
				LastFourDigits: c.CardNumberLast4,
				Brand:          c.CardBrand,
				Status:         c.Status,
				Limit:          c.CreditLimit,
				AvailableLimit: c.AvailableLimit,
				UsedLimit:      c.UsedLimit,
				DueDay:         c.DueDay,
				ClosingDay:     c.BillingDay,
				IsVirtual:      isVirtual,
				CreatedAt:      c.CreatedAt.Format(time.RFC3339),
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{"cards": resp})
	}
}

func cardRequestHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/cards/request")
		defer span.End()

		var apiReq domain.CreditCardRequestBody
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		account, err := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		cardType := "corporate"
		if apiReq.VirtualCard {
			cardType = "virtual"
		}

		req := &domain.CreditCardRequest{
			AccountID:      account.ID,
			CardBrand:      apiReq.PreferredBrand,
			CardType:       cardType,
			DueDay:         apiReq.DueDay,
			RequestedLimit: apiReq.RequestedLimit,
		}

		card, err := bankSvc.RequestCreditCard(ctx, apiReq.CustomerID, req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		isVirtual := card.CardType == "virtual"
		cardResp := &domain.CreditCardAPIResponse{
			ID:             card.ID,
			LastFourDigits: card.CardNumberLast4,
			Brand:          card.CardBrand,
			Status:         card.Status,
			Limit:          card.CreditLimit,
			AvailableLimit: card.AvailableLimit,
			UsedLimit:      card.UsedLimit,
			DueDay:         card.DueDay,
			ClosingDay:     card.BillingDay,
			IsVirtual:      isVirtual,
			CreatedAt:      card.CreatedAt.Format(time.RFC3339),
		}

		deliveryDays := 7
		if isVirtual {
			deliveryDays = 0
		}

		resp := domain.CreditCardRequestResponse{
			RequestID:             card.ID,
			Status:                "approved",
			Card:                  cardResp,
			Message:               "Cartão aprovado com sucesso",
			ApprovedLimit:         card.CreditLimit,
			EstimatedDeliveryDays: deliveryDays,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func cardInvoiceByMonthHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/cards/{cardId}/invoices/{month}")
		defer span.End()

		cardID := chi.URLParam(r, "cardId")
		month := chi.URLParam(r, "month")

		respondWithInvoice(ctx, w, bankSvc, logger, "", cardID, month)
	}
}

func cardInvoiceCurrentHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/credit-cards/{cardId}/invoice")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		cardID := chi.URLParam(r, "cardId")
		currentMonth := time.Now().Format("2006-01")

		respondWithInvoice(ctx, w, bankSvc, logger, customerID, cardID, currentMonth)
	}
}

// respondWithInvoice is the shared logic for both invoice endpoints.
// It fetches the invoice, filters transactions by month, and writes the JSON response.
func respondWithInvoice(ctx context.Context, w http.ResponseWriter, bankSvc *service.BankingService, logger *zap.Logger, customerID, cardID, month string) {
	invoice, err := bankSvc.GetCardInvoiceByMonth(ctx, customerID, cardID, month)
	if err != nil {
		handleServiceError(w, err, logger)
		return
	}

	// Resolve customerID for transaction lookup (may be empty from the by-month endpoint)
	txCustomerID := customerID
	if txCustomerID == "" {
		txCustomerID = invoice.CustomerID
	}

	const maxTransactions = 500
	txns, _ := bankSvc.ListCardTransactions(ctx, txCustomerID, cardID, 1, maxTransactions)

	// Only include transactions from the requested month
	txnResp := make([]domain.InvoiceTransactionResponse, 0, len(txns))
	for _, t := range txns {
		if t.TransactionDate.Format("2006-01") == month {
			txnResp = append(txnResp, buildInvoiceTransactionResponse(t))
		}
	}

	writeJSON(w, http.StatusOK, domain.CreditCardInvoiceAPIResponse{
		ID:             invoice.ID,
		CardID:         invoice.CardID,
		ReferenceMonth: invoice.ReferenceMonth,
		TotalAmount:    invoice.TotalAmount,
		MinimumPayment: invoice.MinimumPayment,
		DueDate:        invoice.DueDate,
		Status:         invoice.Status,
		Transactions:   txnResp,
	})
}

func cardBlockHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/cards/{cardId}/block")
		defer span.End()

		cardID := chi.URLParam(r, "cardId")
		if err := bankSvc.BlockCreditCardByID(ctx, cardID); err != nil {
			handleServiceError(w, err, logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func cardUnblockHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/cards/{cardId}/unblock")
		defer span.End()

		cardID := chi.URLParam(r, "cardId")
		if err := bankSvc.UnblockCreditCardByID(ctx, cardID); err != nil {
			handleServiceError(w, err, logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func cardCancelHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/cards/{cardId}/cancel")
		defer span.End()

		cardID := chi.URLParam(r, "cardId")
		if err := bankSvc.CancelCreditCardByID(ctx, cardID); err != nil {
			handleServiceError(w, err, logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ============================================================
// Invoice Payment Handler
// ============================================================

func invoicePayHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/customers/{customerId}/credit-cards/{cardId}/invoice/pay")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		cardID := chi.URLParam(r, "cardId")

		var req domain.InvoicePayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.PayInvoice(ctx, customerID, cardID, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// buildInvoiceTransactionResponse converts a CreditCardTransaction into an
// InvoiceTransactionResponse, including fee breakdown when original_amount
// is present (e.g. PIX via credit card with installments/fees).
func buildInvoiceTransactionResponse(t domain.CreditCardTransaction) domain.InvoiceTransactionResponse {
	installmentStr := ""
	if t.Installments > 1 {
		installmentStr = fmt.Sprintf("%d/%d", t.CurrentInstallment, t.Installments)
	}

	resp := domain.InvoiceTransactionResponse{
		ID:          t.ID,
		Date:        t.TransactionDate.Format(time.RFC3339),
		Description: t.MerchantName,
		Amount:      t.Amount,
		Installment: installmentStr,
		Category:    t.Category,
	}

	// If original_amount is set and differs from amount, include fee breakdown.
	if t.OriginalAmount != nil && *t.OriginalAmount > 0 {
		resp.OriginalAmount = t.OriginalAmount
		feeAmount := t.Amount - *t.OriginalAmount
		if feeAmount > 0 {
			feeRounded := math.Round(feeAmount*100) / 100
			resp.FeeAmount = &feeRounded
		}
		totalWithFees := t.Amount
		resp.TotalWithFees = &totalWithFees
		// Show the original PIX amount as the main "amount"
		resp.Amount = *t.OriginalAmount
	}

	if t.InstallmentAmount != nil && *t.InstallmentAmount > 0 {
		resp.InstallmentAmount = t.InstallmentAmount
	}

	return resp
}
