package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("handler")

// NewRouter creates the HTTP router with all routes and middleware.
// Routes follow the API contract defined for the PJ Assistant frontend.
func NewRouter(svc *service.Assistant, bankSvc *service.BankingService, authSvc *service.AuthService, metrics *observability.Metrics, logger *zap.Logger) http.Handler {
	r := chi.NewRouter()

	// --- Middleware ---
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(observability.ZapLoggerMiddleware(logger))
	r.Use(observability.TracingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))

	// --- Operational endpoints ---
	r.Get("/healthz", healthzHandler(bankSvc, logger))
	r.Get("/readyz", readyzHandler())
	r.Handle("/metrics", promhttp.Handler())

	// --- API v1 ---
	r.Route("/v1", func(r chi.Router) {

		// =============================================
		// 1. 🤖 Assistente IA
		// POST /v1/assistant/{customerId}
		// =============================================
		r.Post("/assistant/{customerId}", assistantHandler(svc, logger))

		// =============================================
		// 1b. 💬 Chat (alias for assistant)
		// POST /v1/chat
		// =============================================
		r.Post("/chat", chatHandler(svc, logger))

		// =============================================
		// 2. 👤 Cliente
		// GET /v1/customers/{customerId}/profile
		// =============================================
		r.Get("/customers/{customerId}/profile", getProfileHandler(svc, logger))

		// =============================================
		// 3. 💰 Transações
		// GET /v1/customers/{customerId}/transactions
		// GET /v1/customers/{customerId}/transactions/summary
		// =============================================
		r.Get("/customers/{customerId}/transactions", getTransactionsHandler(svc, logger))
		r.Get("/customers/{customerId}/transactions/summary", getTransactionsSummaryHandler(bankSvc, logger))

		// =============================================
		// 4. 📊 Métricas
		// GET /v1/metrics/agent
		// =============================================
		r.Get("/metrics/agent", agentMetricsHandler(metrics, logger))

		// =============================================
		// 5. ⚡ Pix
		// =============================================
		r.Get("/pix/keys/lookup", pixKeyLookupHandler(bankSvc, logger))
		r.Get("/pix/lookup", pixKeyLookupHandler(bankSvc, logger))
		r.Post("/pix/transfer", pixTransferHandler(bankSvc, logger))
		r.Post("/pix/schedule", pixScheduleHandler(bankSvc, logger))
		r.Delete("/pix/schedule/{scheduleId}", pixScheduleDeleteHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/pix/scheduled", pixScheduledListHandler(bankSvc, logger))
		r.Get("/pix/scheduled/{customerId}", pixScheduledListByParamHandler(bankSvc, logger))
		r.Post("/pix/credit-card", pixCreditCardHandler(bankSvc, logger))
		r.Post("/pix/credit", pixCreditCardHandler(bankSvc, logger))
		r.Delete("/pix/keys", pixKeyDeleteByValueHandler(bankSvc, logger))
		r.Get("/pix/receipts/{receiptId}", getPixReceiptHandler(bankSvc, logger))
		r.Get("/pix/transfers/{transferId}/receipt", getPixReceiptByTransferHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/pix/receipts", listPixReceiptsHandler(bankSvc, logger))

		// =============================================
		// 6. 📄 Pagamento de Boletos
		// =============================================
		r.Post("/bills/validate", billsValidateHandler(bankSvc, logger))
		r.Post("/bills/pay", billsPayHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/bills/history", billsHistoryHandler(bankSvc, logger))

		// =============================================
		// 7. 💳 Cartão de Crédito
		// =============================================
		r.Get("/customers/{customerId}/cards", listCardsHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/credit-cards", listCardsHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/credit-limit", creditLimitHandler(bankSvc, logger))
		r.Post("/cards/request", cardRequestHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/credit-cards/request", cardRequestHandler(bankSvc, logger))
		r.Get("/cards/{cardId}/invoices/{month}", cardInvoiceByMonthHandler(bankSvc, logger))
		r.Post("/cards/{cardId}/block", cardBlockHandler(bankSvc, logger))
		r.Post("/cards/{cardId}/unblock", cardUnblockHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/credit-cards/{cardId}/block", cardBlockHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/credit-cards/{cardId}/unblock", cardUnblockHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/credit-cards/{cardId}/invoice", cardInvoiceCurrentHandler(bankSvc, logger))

		// =============================================
		// 8. 📈 Análise Financeira & Débito
		// =============================================
		r.Get("/customers/{customerId}/financial/summary", financialSummaryHandler(bankSvc, logger))
		r.Post("/debit/purchase", debitPurchaseHandler(bankSvc, logger))

		// =============================================
		// Extra internal endpoints
		// =============================================
		r.Get("/customers/{customerId}/accounts", listAccountsHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/accounts/{accountId}", getAccountHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/accounts/{accountId}/balance", getBalanceHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/pix/keys", listPixKeysHandler(bankSvc, logger))
		r.Delete("/customers/{customerId}/pix/keys/{keyId}", deletePixKeyHandler(bankSvc, logger))

		// Favorites
		r.Get("/customers/{customerId}/favorites", listFavoritesHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/favorites", createFavoriteHandler(bankSvc, logger))
		r.Delete("/customers/{customerId}/favorites/{favoriteId}", deleteFavoriteHandler(bankSvc, logger))

		// Transaction Limits
		r.Get("/customers/{customerId}/limits", listLimitsHandler(bankSvc, logger))
		r.Put("/customers/{customerId}/limits/{limitType}", updateLimitHandler(bankSvc, logger))

		// Notifications
		r.Get("/customers/{customerId}/notifications", listNotificationsHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/notifications/{notifId}/read", markNotificationReadHandler(bankSvc, logger))

		// Budgets
		r.Get("/customers/{customerId}/analytics/budgets", listBudgetsHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/analytics/budgets", createBudgetHandler(bankSvc, logger))
		r.Put("/customers/{customerId}/analytics/budgets/{budgetId}", updateBudgetHandler(bankSvc, logger))

		// =============================================
		// Pix Key Registration
		// =============================================
		r.Post("/pix/keys/register", pixKeyRegisterHandler(bankSvc, logger))

		// =============================================
		// Invoice Payment
		// =============================================
		r.Post("/customers/{customerId}/credit-cards/{cardId}/invoice/pay", invoicePayHandler(bankSvc, logger))

		// =============================================
		// 🛠 Dev Tools (testing helpers)
		// =============================================
		r.Post("/dev/add-balance", devAddBalanceHandler(bankSvc, logger))
		r.Post("/dev/set-credit-limit", devSetCreditLimitHandler(bankSvc, logger))
		r.Post("/dev/generate-transactions", devGenerateTransactionsHandler(bankSvc, logger))
		r.Post("/dev/add-card-purchase", devAddCardPurchaseHandler(bankSvc, logger))

		// =============================================
		// 9. 🔐 Autenticação
		// =============================================
		r.Route("/auth", func(r chi.Router) {
			if authSvc == nil {
				r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					writeError(w, http.StatusServiceUnavailable, "auth service unavailable: Supabase not configured")
				}))
				return
			}
			// Public routes
			r.Post("/register", authRegisterHandler(authSvc, logger))
			r.Post("/login", authLoginHandler(authSvc, logger))
			r.Post("/refresh", authRefreshHandler(authSvc, logger))
			r.Post("/password/reset-request", authPasswordResetRequestHandler(authSvc, logger))
			r.Post("/password/reset-confirm", authPasswordResetConfirmHandler(authSvc, logger))

			// Protected routes
			r.Group(func(r chi.Router) {
				r.Use(JWTAuthMiddleware(authSvc, logger))
				r.Post("/logout", authLogoutHandler(authSvc, logger))
				r.Put("/password", authChangePasswordHandler(authSvc, logger))
			})
		})

		// =============================================
		// 10. 👤 Profile & Representative (protected)
		// =============================================
		if authSvc != nil {
			r.Group(func(r chi.Router) {
				r.Use(JWTAuthMiddleware(authSvc, logger))
				r.Put("/customers/{customerId}/profile", updateProfileHandler(authSvc, logger))
				r.Put("/customers/{customerId}/representative", updateRepresentativeHandler(authSvc, logger))
			})
		}
	})

	return r
}

// ============================================================
// 1. Assistente IA — POST /v1/assistant/{customerId}
// ============================================================

func assistantHandler(svc *service.Assistant, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/assistant/{customerId}")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		if customerID == "" {
			writeError(w, http.StatusBadRequest, "customer_id is required")
			return
		}
		span.SetAttributes(attribute.String("customer.id", customerID))

		var req domain.AssistantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		start := time.Now()
		result, err := svc.GetAssistantResponse(ctx, customerID, req.Message)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		convID := req.ConversationID
		if convID == "" {
			convID = uuid.New().String()
		}

		resp := domain.AssistantResponse{
			ConversationID: convID,
			Message: &domain.AssistantMessage{
				ID:        uuid.New().String(),
				Role:      "assistant",
				Content:   result.Recommendation.Answer,
				Timestamp: time.Now().Format(time.RFC3339),
				Metadata: &domain.MessageMetadata{
					ToolsUsed: result.Recommendation.ToolsExecuted,
					TokenUsage: &domain.TokenUsage{
						PromptTokens:     result.Recommendation.TokensUsed.PromptTokens,
						CompletionTokens: result.Recommendation.TokensUsed.CompletionTokens,
						TotalTokens:      result.Recommendation.TokensUsed.TotalTokens,
					},
					LatencyMs: latencyMs,
					Reasoning: result.Recommendation.Reasoning,
				},
			},
			Profile: result.Profile,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// chatHandler is an alias for the assistant endpoint that accepts customerId in the body.
// Matches the document spec: POST /v1/chat with { customerId, message, conversationId }
func chatHandler(svc *service.Assistant, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/chat")
		defer span.End()

		var req struct {
			CustomerID     string `json:"customerId"`
			Message        string `json:"message"`
			ConversationID string `json:"conversationId,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.CustomerID == "" {
			writeError(w, http.StatusBadRequest, "customerId is required")
			return
		}
		span.SetAttributes(attribute.String("customer.id", req.CustomerID))

		start := time.Now()
		result, err := svc.GetAssistantResponse(ctx, req.CustomerID, req.Message)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		convID := req.ConversationID
		if convID == "" {
			convID = uuid.New().String()
		}

		resp := domain.AssistantResponse{
			ConversationID: convID,
			Message: &domain.AssistantMessage{
				ID:        uuid.New().String(),
				Role:      "assistant",
				Content:   result.Recommendation.Answer,
				Timestamp: time.Now().Format(time.RFC3339),
				Metadata: &domain.MessageMetadata{
					ToolsUsed: result.Recommendation.ToolsExecuted,
					TokenUsage: &domain.TokenUsage{
						PromptTokens:     result.Recommendation.TokensUsed.PromptTokens,
						CompletionTokens: result.Recommendation.TokensUsed.CompletionTokens,
						TotalTokens:      result.Recommendation.TokensUsed.TotalTokens,
					},
					LatencyMs: latencyMs,
					Reasoning: result.Recommendation.Reasoning,
				},
			},
			Profile: result.Profile,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ============================================================
// 2. Cliente — GET /v1/customers/{customerId}/profile
// ============================================================

func getProfileHandler(svc *service.Assistant, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/profile")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		profile, err := svc.GetProfile(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

// ============================================================
// 3. Transações
// ============================================================

func getTransactionsHandler(svc *service.Assistant, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/transactions")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		transactions, err := svc.GetTransactions(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		// Filter by type(s) if provided — e.g. ?type=pix_sent,pix_received
		if typeFilter := r.URL.Query().Get("type"); typeFilter != "" {
			allowedTypes := make(map[string]bool)
			for _, t := range strings.Split(typeFilter, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					allowedTypes[t] = true
				}
			}
			if len(allowedTypes) > 0 {
				filtered := make([]domain.Transaction, 0, len(transactions))
				for _, tx := range transactions {
					if allowedTypes[tx.Type] {
						filtered = append(filtered, tx)
					}
				}
				transactions = filtered
			}
		}

		// Filter by category if provided — e.g. ?category=pix,pix_credito
		if catFilter := r.URL.Query().Get("category"); catFilter != "" {
			allowedCats := make(map[string]bool)
			for _, c := range strings.Split(catFilter, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					allowedCats[c] = true
				}
			}
			if len(allowedCats) > 0 {
				filtered := make([]domain.Transaction, 0, len(transactions))
				for _, tx := range transactions {
					if allowedCats[tx.Category] {
						filtered = append(filtered, tx)
					}
				}
				transactions = filtered
			}
		}

		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit < len(transactions) {
				transactions = transactions[:limit]
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"transactions": transactions})
	}
}

func getTransactionsSummaryHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/transactions/summary")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		summary, err := bankSvc.GetTransactionSummary(ctx, customerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

// ============================================================
// 4. Métricas & Health
// ============================================================

func healthzHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now().Format(time.RFC3339)

		services := []domain.ServiceHealth{
			{Name: "bfa-api", Status: "healthy", LatencyMs: 0, UptimePercent: 99.99, LastChecked: now},
		}

		if bankSvc != nil {
			start := time.Now()
			_, err := bankSvc.ListAccounts(ctx, "health-check")
			latency := time.Since(start).Milliseconds()
			status := "healthy"
			if err != nil {
				status = "degraded"
			}
			services = append(services, domain.ServiceHealth{
				Name: "supabase", Status: status, LatencyMs: latency,
				UptimePercent: 99.9, LastChecked: now,
			})
		}

		overallStatus := "healthy"
		for _, s := range services {
			if s.Status == "unhealthy" {
				overallStatus = "unhealthy"
				break
			}
			if s.Status == "degraded" {
				overallStatus = "degraded"
			}
		}

		writeJSON(w, http.StatusOK, domain.HealthStatus{
			Status:   overallStatus,
			Services: services,
		})
	}
}

func agentMetricsHandler(metrics *observability.Metrics, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := metrics.GetAgentSnapshot()
		writeJSON(w, http.StatusOK, snapshot)
	}
}

// ============================================================
// 5. PIX
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

// pixScheduledListByParamHandler is an alias for pixScheduledListHandler
// using the URL pattern GET /v1/pix/scheduled/{customerId}
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

		// Default installments to 1 to avoid division by zero
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

		// Calculate fees BEFORE calling service so limits are validated correctly
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
// 5b. ⚡ Pix Receipts (Comprovantes)
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
// 6. Pagamento de Boletos
// ============================================================

func billsValidateHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/bills/validate")
		defer span.End()

		var body struct {
			Barcode string `json:"barcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		valReq := &domain.BarcodeValidationRequest{
			InputMethod:   "typed",
			DigitableLine: body.Barcode,
			Barcode:       body.Barcode,
		}

		result, err := bankSvc.ValidateBarcode(ctx, valReq)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := domain.BarcodeValidationAPIResponse{Valid: result.IsValid}
		if result.IsValid {
			billType := result.BillType
			switch billType {
			case "bank_slip":
				billType = "boleto"
			case "utility":
				billType = "concessionaria"
			}
			resp.Data = &domain.BarcodeData{
				Barcode:       result.Barcode,
				DigitableLine: result.DigitableLine,
				Type:          billType,
				Amount:        result.Amount,
				DueDate:       result.DueDate,
				Beneficiary:   result.BeneficiaryName,
				Bank:          result.BankCode,
				TotalAmount:   result.Amount,
			}
		} else {
			if len(result.ValidationErrors) > 0 {
				resp.ErrorMessage = result.ValidationErrors[0]
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func billsPayHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/bills/pay")
		defer span.End()

		var apiReq domain.BillPaymentAPIRequest
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		account, err := bankSvc.GetPrimaryAccount(ctx, apiReq.CustomerID)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		req := &domain.BillPaymentRequest{
			IdempotencyKey: uuid.New().String(),
			AccountID:      account.ID,
			InputMethod:    apiReq.InputMethod,
			DigitableLine:  apiReq.Barcode,
			Barcode:        apiReq.Barcode,
			ScheduledDate:  apiReq.PaymentDate,
		}

		payment, err := bankSvc.PayBill(ctx, apiReq.CustomerID, req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := domain.BillPaymentAPIResponse{
			TransactionID:  payment.ID,
			Status:         payment.Status,
			Amount:         payment.FinalAmount,
			Beneficiary:    payment.BeneficiaryName,
			DueDate:        payment.DueDate,
			PaymentDate:    payment.PaymentDate,
			Authentication: payment.IdempotencyKey,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func billsHistoryHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/bills/history")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		page, pageSize := parsePagination(r)

		payments, err := bankSvc.ListBillPayments(ctx, customerID, page, pageSize)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		resp := make([]domain.BillPaymentAPIResponse, 0, len(payments))
		for _, p := range payments {
			resp = append(resp, domain.BillPaymentAPIResponse{
				TransactionID:  p.ID,
				Status:         p.Status,
				Amount:         p.FinalAmount,
				Beneficiary:    p.BeneficiaryName,
				DueDate:        p.DueDate,
				PaymentDate:    p.PaymentDate,
				Authentication: p.IdempotencyKey,
			})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

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

// cardInvoiceCurrentHandler returns the current/latest invoice for a card.
// Matches: GET /v1/customers/{customerId}/credit-cards/{cardId}/invoice
func cardInvoiceCurrentHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/credit-cards/{cardId}/invoice")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		cardID := chi.URLParam(r, "cardId")

		// Use current month
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
// 8. Análise Financeira & Débito
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

func debitPurchaseHandler(bankSvc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/debit/purchase")
		defer span.End()

		var apiReq domain.DebitPurchaseRequest
		if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := bankSvc.CreateDebitPurchase(ctx, apiReq.CustomerID, &apiReq)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}

// ============================================================
// Extra Internal Endpoints
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
		// Build response with formatted display values
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

// formatKeyValue returns a human-readable formatted version of a pix key value.
func formatKeyValue(keyType, value string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	switch keyType {
	case "cnpj":
		if len(digits) == 14 {
			return fmt.Sprintf("%s.%s.%s/%s-%s", digits[:2], digits[2:5], digits[5:8], digits[8:12], digits[12:14])
		}
	case "cpf":
		if len(digits) == 11 {
			return fmt.Sprintf("%s.%s.%s-%s", digits[:3], digits[3:6], digits[6:9], digits[9:11])
		}
	case "phone":
		if len(digits) == 11 {
			return fmt.Sprintf("(%s) %s-%s", digits[:2], digits[2:7], digits[7:11])
		} else if len(digits) == 13 { // +55...
			return fmt.Sprintf("+%s (%s) %s-%s", digits[:2], digits[2:4], digits[4:9], digits[9:13])
		}
	}
	return value
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

func creditLimitHandler(svc *service.BankingService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /v1/customers/{customerId}/credit-limit")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")
		limit, err := svc.GetCreditLimit(ctx, customerID)
		if err != nil {
			// If no cards found, return 0
			writeJSON(w, http.StatusOK, map[string]any{"creditLimit": 0})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"creditLimit": limit})
	}
}

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

// ============================================================
// 9. Autenticação
// ============================================================

func authRegisterHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/register")
		defer span.End()

		var req domain.RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.Register(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

func authLoginHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/login")
		defer span.End()

		var req domain.LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.Login(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func authRefreshHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/refresh")
		defer span.End()

		var req domain.RefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.Refresh(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func authLogoutHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/logout")
		defer span.End()

		customerID := CustomerIDFromContext(ctx)
		if customerID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if err := authSvc.Logout(ctx, customerID); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func authPasswordResetRequestHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/password/reset-request")
		defer span.End()

		var req domain.PasswordResetRequestBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.PasswordResetRequest(ctx, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func authPasswordResetConfirmHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "POST /v1/auth/password/reset-confirm")
		defer span.End()

		var req domain.PasswordResetConfirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := authSvc.PasswordResetConfirm(ctx, &req); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, domain.SuccessResponse{Message: "Senha redefinida com sucesso"})
	}
}

func authChangePasswordHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /v1/auth/password")
		defer span.End()

		customerID := CustomerIDFromContext(ctx)
		if customerID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req domain.ChangePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := authSvc.ChangePassword(ctx, customerID, &req); err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, domain.SuccessResponse{Message: "Senha alterada com sucesso"})
	}
}

// ============================================================
// 10. Profile & Representative
// ============================================================

func updateProfileHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /v1/customers/{customerId}/profile")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")

		var req domain.UpdateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.UpdateProfile(ctx, customerID, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func updateRepresentativeHandler(authSvc *service.AuthService, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "PUT /v1/customers/{customerId}/representative")
		defer span.End()

		customerID := chi.URLParam(r, "customerId")

		var req domain.UpdateRepresentativeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp, err := authSvc.UpdateRepresentative(ctx, customerID, &req)
		if err != nil {
			handleServiceError(w, err, logger)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ============================================================
// Probes
// ============================================================

func readyzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// ============================================================
// Helpers
// ============================================================

func handleServiceError(w http.ResponseWriter, err error, logger *zap.Logger) {
	var notFound *domain.ErrNotFound
	var circuitOpen *domain.ErrCircuitOpen
	var timeout *domain.ErrTimeout
	var validation *domain.ErrValidation
	var insufficientFunds *domain.ErrInsufficientFunds
	var limitExceeded *domain.ErrLimitExceeded
	var duplicate *domain.ErrDuplicate
	var forbidden *domain.ErrForbidden
	var invalidBarcode *domain.ErrInvalidBarcode
	var unauthorized *domain.ErrUnauthorized
	var accountBlocked *domain.ErrAccountBlocked
	var conflict *domain.ErrConflict
	var invalidCode *domain.ErrInvalidCode

	switch {
	case errors.As(err, &notFound):
		logger.Debug("not found", zap.String("error", err.Error()))
		writeError(w, http.StatusNotFound, err.Error())
	case errors.As(err, &circuitOpen):
		logger.Error("circuit breaker open", zap.Error(err))
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.As(err, &timeout):
		logger.Error("request timeout", zap.Error(err))
		writeError(w, http.StatusGatewayTimeout, err.Error())
	case errors.As(err, &validation):
		logger.Debug("validation error", zap.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &insufficientFunds):
		logger.Warn("insufficient funds",
			zap.Float64("available", insufficientFunds.Available),
			zap.Float64("required", insufficientFunds.Required),
		)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.As(err, &limitExceeded):
		logger.Warn("limit exceeded", zap.String("error", err.Error()))
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.As(err, &duplicate):
		logger.Debug("duplicate resource", zap.String("error", err.Error()))
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &forbidden):
		logger.Warn("forbidden access", zap.String("error", err.Error()))
		writeError(w, http.StatusForbidden, err.Error())
	case errors.As(err, &invalidBarcode):
		logger.Debug("invalid barcode", zap.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &unauthorized):
		logger.Warn("unauthorized", zap.String("error", err.Error()))
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.As(err, &accountBlocked):
		logger.Warn("account blocked", zap.String("status", accountBlocked.Status))
		writeError(w, http.StatusForbidden, err.Error())
	case errors.As(err, &conflict):
		logger.Debug("conflict", zap.String("error", err.Error()))
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &invalidCode):
		logger.Warn("invalid verification code")
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		logger.Error("unhandled error", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
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

// ============================================================
// Helper functions
// ============================================================

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func parsePagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}
	return
}
