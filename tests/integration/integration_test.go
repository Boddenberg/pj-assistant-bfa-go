package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/handler"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/cache"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/client"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"go.uber.org/zap"
)

// TestIntegration_FullFlow spins up mock external services and tests the full request flow.
func TestIntegration_FullFlow(t *testing.T) {
	// --- Mock Profile API ---
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profile := domain.CustomerProfile{
			CustomerID:     "cust-integration-1",
			Name:           "Empresa Integration Test",
			Document:       "12.345.678/0001-90",
			Segment:        "middle_market",
			MonthlyRevenue: 150000,
			AccountAge:     36,
			CreditScore:    750,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)
	}))
	defer profileServer.Close()

	// --- Mock Transactions API ---
	txServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transactions := []domain.Transaction{
			{ID: "tx-1", Date: time.Now(), Amount: 50000, Type: "credit", Category: "revenue"},
			{ID: "tx-2", Date: time.Now(), Amount: -20000, Type: "debit", Category: "supplier"},
			{ID: "tx-3", Date: time.Now(), Amount: -5000, Type: "debit", Category: "utilities"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(transactions)
	}))
	defer txServer.Close()

	// --- Mock Agent API ---
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := domain.AgentResponse{
			Answer:     "Based on your strong financials, we recommend expanding your credit line.",
			Reasoning:  "Positive cash flow, high credit score, established relationship.",
			Confidence: 0.95,
			Sources:    []string{"politica_credito.txt"},
			TokensUsed: domain.TokenUsage{PromptTokens: 800, CompletionTokens: 200, TotalTokens: 1000},
			ToolsExecuted: []string{"rag_retrieval", "financial_analysis", "llm_synthesis"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer agentServer.Close()

	// --- Build service ---
	logger := zap.NewNop()
	metrics := observability.NewMetrics()
	cb := resilience.NewCircuitBreaker("test")
	cfg := resilience.Config{MaxRetries: 1, InitialBackoff: 10 * time.Millisecond, MaxConcurrency: 10}
	httpClient := &http.Client{Timeout: 5 * time.Second}

	svc := service.NewAssistant(
		client.NewProfileClient(httpClient, profileServer.URL, cb, cfg),
		client.NewTransactionsClient(httpClient, txServer.URL, cb, cfg),
		client.NewAgentClient(httpClient, agentServer.URL, cb, cfg),
		cache.New[any](5*time.Minute),
		metrics,
		logger,
	)

	router := handler.NewRouter(svc, nil, nil, metrics, logger)

	// --- Execute request ---
	body, _ := json.Marshal(domain.AssistantRequest{Message: "What is my financial status?"})
	req := httptest.NewRequest(http.MethodPost, "/v1/assistant/cust-integration-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// --- Assertions ---
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var result domain.AssistantResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.ConversationID == "" {
		t.Error("expected conversationId to be present")
	}
	if result.Profile == nil {
		t.Fatal("expected profile to be present")
	}
	if result.Profile.Name != "Empresa Integration Test" {
		t.Errorf("expected name 'Empresa Integration Test', got '%s'", result.Profile.Name)
	}
	if result.Message == nil {
		t.Fatal("expected message to be present")
	}
	if result.Message.Content == "" {
		t.Error("expected message content to be non-empty")
	}

	fmt.Printf("✅ Integration test passed: %s\n",
		result.Message.Content[:50])
}

// TestIntegration_ProfileNotFound tests 404 handling from Profile API.
func TestIntegration_ProfileNotFound(t *testing.T) {
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer profileServer.Close()

	txServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]domain.Transaction{})
	}))
	defer txServer.Close()

	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(domain.AgentResponse{})
	}))
	defer agentServer.Close()

	logger := zap.NewNop()
	metrics := observability.NewMetrics()
	cb := resilience.NewCircuitBreaker("test-404")
	cfg := resilience.Config{MaxRetries: 0, InitialBackoff: 10 * time.Millisecond, MaxConcurrency: 10}
	httpClient := &http.Client{Timeout: 5 * time.Second}

	svc := service.NewAssistant(
		client.NewProfileClient(httpClient, profileServer.URL, cb, cfg),
		client.NewTransactionsClient(httpClient, txServer.URL, cb, cfg),
		client.NewAgentClient(httpClient, agentServer.URL, cb, cfg),
		cache.New[any](5*time.Minute),
		metrics,
		logger,
	)

	router := handler.NewRouter(svc, nil, nil, metrics, logger)

	body, _ := json.Marshal(domain.AssistantRequest{Message: "test"})
	req := httptest.NewRequest(http.MethodPost, "/v1/assistant/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should return an error (not 200)
	if rec.Code == http.StatusOK {
		t.Error("expected non-200 for missing profile")
	}
}
