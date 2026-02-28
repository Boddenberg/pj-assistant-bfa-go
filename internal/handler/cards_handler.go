package handler

import (
	"encoding/json"
	"fmt"
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

		invoice, err := bankSvc.GetCardInvoiceByMonth(ctx, "", cardID, month)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		txns, _ := bankSvc.ListCardTransactions(ctx, invoice.CustomerID, cardID, 1, 100)

		txnResp := make([]domain.InvoiceTransactionResponse, 0, len(txns))
		for _, t := range txns {
			installmentStr := ""
			if t.Installments > 1 {
				installmentStr = fmt.Sprintf("%d/%d", t.CurrentInstallment, t.Installments)
			}
			txnResp = append(txnResp, domain.InvoiceTransactionResponse{
				ID:          t.ID,
				Date:        t.TransactionDate.Format(time.RFC3339),
				Description: t.MerchantName,
				Amount:      t.Amount,
				Installment: installmentStr,
				Category:    t.Category,
			})
		}

		resp := domain.CreditCardInvoiceAPIResponse{
			ID:             invoice.ID,
			CardID:         invoice.CardID,
			ReferenceMonth: invoice.ReferenceMonth,
			TotalAmount:    invoice.TotalAmount,
			MinimumPayment: invoice.MinimumPayment,
			DueDate:        invoice.DueDate,
			Status:         invoice.Status,
			Transactions:   txnResp,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func cardInvoiceCurrentHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/credit-cards/{cardId}/invoice")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		cardID := chi.URLParam(r, "cardId")

		currentMonth := time.Now().Format("2006-01")
		invoice, err := bankSvc.GetCardInvoiceByMonth(ctx, customerID, cardID, currentMonth)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		txns, _ := bankSvc.ListCardTransactions(ctx, customerID, cardID, 1, 100)

		txnResp := make([]domain.InvoiceTransactionResponse, 0, len(txns))
		for _, t := range txns {
			installmentStr := ""
			if t.Installments > 1 {
				installmentStr = fmt.Sprintf("%d/%d", t.CurrentInstallment, t.Installments)
			}
			txnResp = append(txnResp, domain.InvoiceTransactionResponse{
				ID:          t.ID,
				Date:        t.TransactionDate.Format(time.RFC3339),
				Description: t.MerchantName,
				Amount:      t.Amount,
				Installment: installmentStr,
				Category:    t.Category,
			})
		}

		resp := domain.CreditCardInvoiceAPIResponse{
			ID:             invoice.ID,
			CardID:         invoice.CardID,
			ReferenceMonth: invoice.ReferenceMonth,
			TotalAmount:    invoice.TotalAmount,
			MinimumPayment: invoice.MinimumPayment,
			DueDate:        invoice.DueDate,
			Status:         invoice.Status,
			Transactions:   txnResp,
		}

		writeJSON(w, http.StatusOK, resp)
	}
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
