package handler

import (
	"encoding/json"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ============================================================
// 8. Análise Financeira
// ============================================================

func financialSummaryHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/financial/summary")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		period := r.URL.Query().Get("period")
		if period == "" {
			period = "30d"
		}

		summary, err := bankSvc.GetFinancialSummary(ctx, customerID, period)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

// ============================================================
// Favorites
// ============================================================

func listFavoritesHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /favorites")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		favorites, err := svc.ListFavorites(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, favorites)
	}
}

func createFavoriteHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /favorites")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		var fav domain.Favorite
		if err := json.NewDecoder(r.Body).Decode(&fav); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		fav.CustomerID = customerID
		created, err := svc.CreateFavorite(ctx, &fav)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func deleteFavoriteHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "DELETE /favorites/{favoriteId}")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		favoriteID := chi.URLParam(r, "favoriteId")
		if err := svc.DeleteFavorite(ctx, customerID, favoriteID); err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, domain.SuccessResponse{Message: "favorite deleted"})
	}
}

// ============================================================
// Transaction Limits
// ============================================================

func listLimitsHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /limits")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		limits, err := svc.ListLimits(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, limits)
	}
}

func updateLimitHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /limits/{limitType}")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		limitType := chi.URLParam(r, "limitType")
		var limit domain.TransactionLimit
		if err := json.NewDecoder(r.Body).Decode(&limit); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		limit.CustomerID = customerID
		limit.TransactionType = limitType
		updated, err := svc.UpdateLimit(ctx, &limit)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// ============================================================
// Notifications
// ============================================================

func listNotificationsHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /notifications")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		page, pageSize := parsePagination(r)
		unreadOnly := r.URL.Query().Get("unread") == "true"
		notifications, err := svc.ListNotifications(ctx, customerID, unreadOnly, page, pageSize)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, notifications)
	}
}

func markNotificationReadHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /notifications/{notifId}/read")
		defer span.End()
		notifID := chi.URLParam(r, "notifId")
		if err := svc.MarkNotificationRead(ctx, notifID); err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, domain.SuccessResponse{Message: "notification marked as read"})
	}
}

// ============================================================
// Budgets
// ============================================================

func listBudgetsHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /analytics/budgets")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		budgets, err := svc.ListBudgets(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, budgets)
	}
}

func createBudgetHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /analytics/budgets")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		var budget domain.SpendingBudget
		if err := json.NewDecoder(r.Body).Decode(&budget); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		budget.CustomerID = customerID
		created, err := svc.CreateBudget(ctx, &budget)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func updateBudgetHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /analytics/budgets/{budgetId}")
		defer span.End()
		customerID := chi.URLParam(r, "customerId")
		budgetID := chi.URLParam(r, "budgetId")
		var budget domain.SpendingBudget
		if err := json.NewDecoder(r.Body).Decode(&budget); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		budget.ID = budgetID
		budget.CustomerID = customerID
		updated, err := svc.UpdateBudget(ctx, &budget)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}
