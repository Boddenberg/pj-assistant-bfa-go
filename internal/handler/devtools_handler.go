package handler

import (
	"encoding/json"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"go.uber.org/zap"
)

// ============================================================
// Dev Tools Handlers
// ============================================================

func devAddBalanceHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/dev/add-balance")
		defer span.End()

		var req domain.DevAddBalanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.DevAddBalance(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func devSetCreditLimitHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/dev/set-credit-limit")
		defer span.End()

		var req domain.DevSetCreditLimitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.DevSetCreditLimit(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func devGenerateTransactionsHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/dev/generate-transactions")
		defer span.End()

		var req domain.DevGenerateTransactionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.DevGenerateTransactions(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func devAddCardPurchaseHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/dev/add-card-purchase")
		defer span.End()

		var req domain.DevAddCardPurchaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.DevAddCardPurchase(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
