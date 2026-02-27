package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/cache"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/service"

	"go.uber.org/zap"
)

// --- Mocks ---

type mockProfileClient struct {
	profile *domain.CustomerProfile
	err     error
}

func (m *mockProfileClient) GetProfile(_ context.Context, _ string) (*domain.CustomerProfile, error) {
	return m.profile, m.err
}

type mockTransactionsClient struct {
	transactions []domain.Transaction
	err          error
}

func (m *mockTransactionsClient) GetTransactions(_ context.Context, _ string) ([]domain.Transaction, error) {
	return m.transactions, m.err
}

type mockAgentClient struct {
	response *domain.AgentResponse
	err      error
}

func (m *mockAgentClient) Call(_ context.Context, _ *domain.AgentRequest) (*domain.AgentResponse, error) {
	return m.response, m.err
}

// --- Tests ---

func TestGetAssistantResponse_Success(t *testing.T) {
	profile := &domain.CustomerProfile{
		CustomerID: "cust-123",
		Name:       "Empresa XPTO",
		Document:   "12.345.678/0001-90",
		Segment:    "middle_market",
	}

	transactions := []domain.Transaction{
		{ID: "tx-1", Amount: 1000, Type: "credit", Category: "revenue"},
		{ID: "tx-2", Amount: -500, Type: "debit", Category: "supplier"},
	}

	agentResp := &domain.AgentResponse{
		Answer:     "Based on your financial profile, I recommend...",
		Reasoning:  "Analysis of cash flow shows healthy patterns.",
		Confidence: 0.92,
		TokensUsed: domain.TokenUsage{PromptTokens: 500, CompletionTokens: 200, TotalTokens: 700},
	}

	svc := service.NewAssistant(
		&mockProfileClient{profile: profile},
		&mockTransactionsClient{transactions: transactions},
		&mockAgentClient{response: agentResp},
		cache.New[any](5*time.Minute),
		observability.NewMetrics(),
		zap.NewNop(),
	)

	result, err := svc.GetAssistantResponse(context.Background(), "cust-123", "What are my finances?")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.CustomerID != "cust-123" {
		t.Errorf("expected customer_id 'cust-123', got '%s'", result.CustomerID)
	}
	if result.Profile.Name != "Empresa XPTO" {
		t.Errorf("expected profile name 'Empresa XPTO', got '%s'", result.Profile.Name)
	}
	if result.Recommendation.Confidence != 0.92 {
		t.Errorf("expected confidence 0.92, got %f", result.Recommendation.Confidence)
	}
}

func TestGetAssistantResponse_ProfileError(t *testing.T) {
	svc := service.NewAssistant(
		&mockProfileClient{err: errors.New("connection refused")},
		&mockTransactionsClient{transactions: []domain.Transaction{}},
		&mockAgentClient{response: &domain.AgentResponse{}},
		cache.New[any](5*time.Minute),
		observability.NewMetrics(),
		zap.NewNop(),
	)

	_, err := svc.GetAssistantResponse(context.Background(), "cust-123", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetAssistantResponse_TransactionsError(t *testing.T) {
	svc := service.NewAssistant(
		&mockProfileClient{profile: &domain.CustomerProfile{CustomerID: "cust-123"}},
		&mockTransactionsClient{err: errors.New("timeout")},
		&mockAgentClient{response: &domain.AgentResponse{}},
		cache.New[any](5*time.Minute),
		observability.NewMetrics(),
		zap.NewNop(),
	)

	_, err := svc.GetAssistantResponse(context.Background(), "cust-123", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetAssistantResponse_AgentError(t *testing.T) {
	svc := service.NewAssistant(
		&mockProfileClient{profile: &domain.CustomerProfile{CustomerID: "cust-123"}},
		&mockTransactionsClient{transactions: []domain.Transaction{}},
		&mockAgentClient{err: errors.New("agent unavailable")},
		cache.New[any](5*time.Minute),
		observability.NewMetrics(),
		zap.NewNop(),
	)

	_, err := svc.GetAssistantResponse(context.Background(), "cust-123", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetAssistantResponse_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	svc := service.NewAssistant(
		&mockProfileClient{profile: &domain.CustomerProfile{}},
		&mockTransactionsClient{transactions: []domain.Transaction{}},
		&mockAgentClient{response: &domain.AgentResponse{}},
		cache.New[any](5*time.Minute),
		observability.NewMetrics(),
		zap.NewNop(),
	)

	_, err := svc.GetAssistantResponse(ctx, "cust-123", "test")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
