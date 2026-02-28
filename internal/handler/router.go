package handler

import (
	"net/http"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
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
		// 1. Assistente IA
		// =============================================
		r.Post("/assistant/{customerId}", assistantHandler(svc, logger))

		// =============================================
		// 1b. Chat (alias for assistant)
		// =============================================
		r.Post("/chat", chatHandler(svc, logger))

		// =============================================
		// 2. Cliente
		// =============================================
		r.Get("/customers/{customerId}/profile", getProfileHandler(svc, logger))

		// =============================================
		// 3. Transações
		// =============================================
		r.Get("/customers/{customerId}/transactions", getTransactionsHandler(svc, logger))
		r.Get("/customers/{customerId}/transactions/summary", getTransactionsSummaryHandler(bankSvc, logger))

		// =============================================
		// 4. Métricas
		// =============================================
		r.Get("/metrics/agent", agentMetricsHandler(metrics, logger))

		// =============================================
		// 5. Pix
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
		// 6. Pagamento de Boletos
		// =============================================
		r.Post("/bills/validate", billsValidateHandler(bankSvc, logger))
		r.Post("/bills/pay", billsPayHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/bills/history", billsHistoryHandler(bankSvc, logger))

		// =============================================
		// 7. Cartão de Crédito
		// =============================================
		r.Get("/customers/{customerId}/cards", listCardsHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/credit-cards", listCardsHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/credit-limit", creditLimitHandler(bankSvc, logger))
		r.Post("/cards/request", cardRequestHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/credit-cards/request", cardRequestHandler(bankSvc, logger))
		r.Get("/cards/{cardId}/invoices/{month}", cardInvoiceByMonthHandler(bankSvc, logger))
		r.Post("/cards/{cardId}/block", cardBlockHandler(bankSvc, logger))
		r.Post("/cards/{cardId}/unblock", cardUnblockHandler(bankSvc, logger))
		r.Post("/cards/{cardId}/cancel", cardCancelHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/credit-cards/{cardId}/block", cardBlockHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/credit-cards/{cardId}/unblock", cardUnblockHandler(bankSvc, logger))
		r.Post("/customers/{customerId}/credit-cards/{cardId}/cancel", cardCancelHandler(bankSvc, logger))
		r.Get("/customers/{customerId}/credit-cards/{cardId}/invoice", cardInvoiceCurrentHandler(bankSvc, logger))

		// =============================================
		// 8. Análise Financeira & Débito
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
		// Dev Tools (testing helpers)
		// =============================================
		r.Post("/dev/add-balance", devAddBalanceHandler(bankSvc, logger))
		r.Post("/dev/set-credit-limit", devSetCreditLimitHandler(bankSvc, logger))
		r.Post("/dev/generate-transactions", devGenerateTransactionsHandler(bankSvc, logger))
		r.Post("/dev/add-card-purchase", devAddCardPurchaseHandler(bankSvc, logger))
		r.Post("/dev/card-purchase", devAddCardPurchaseHandler(bankSvc, logger))

		// =============================================
		// 9. Autenticação
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
		// 10. Profile & Representative (protected)
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
// Operational handlers (healthz, readyz, agent metrics)
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

func readyzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
