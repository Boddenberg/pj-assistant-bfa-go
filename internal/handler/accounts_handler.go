package handler

import (
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ============================================================
// Accounts Handlers
// ============================================================

func listAccountsHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /accounts")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		accounts, err := svc.ListAccounts(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, accounts)
	}
}

func getAccountHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /accounts/{accountId}")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		accountID := chi.URLParam(r, "accountId")
		account, err := svc.GetAccount(ctx, customerID, accountID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, account)
	}
}

func getBalanceHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /accounts/{accountId}/balance")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		accountID := chi.URLParam(r, "accountId")
		account, err := svc.GetAccount(ctx, customerID, accountID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"account_id":        account.ID,
			"balance":           account.Balance,
			"available_balance": account.AvailableBalance,
			"overdraft_limit":   account.OverdraftLimit,
			"currency":          account.Currency,
		})
	}
}
