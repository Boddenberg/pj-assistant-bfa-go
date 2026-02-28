package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

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
