package service

import (
	"context"
	"fmt"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/observability"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var tracer = otel.Tracer("service/assistant")

// Assistant orchestrates calls to Profile, Transactions and Agent APIs.
type Assistant struct {
	profileClient      port.ProfileFetcher
	transactionsClient port.TransactionsFetcher
	agentClient        port.AgentCaller
	cache              port.Cache[any]
	metrics            *observability.Metrics
	logger             *zap.Logger
}

// NewAssistant creates the assistant service with all dependencies injected.
func NewAssistant(
	profile port.ProfileFetcher,
	transactions port.TransactionsFetcher,
	agent port.AgentCaller,
	cache port.Cache[any],
	metrics *observability.Metrics,
	logger *zap.Logger,
) *Assistant {
	return &Assistant{
		profileClient:      profile,
		transactionsClient: transactions,
		agentClient:        agent,
		cache:              cache,
		metrics:            metrics,
		logger:             logger,
	}
}

// GetProfile fetches the customer profile (used by the dedicated /profile route).
func (a *Assistant) GetProfile(ctx context.Context, customerID string) (*domain.CustomerProfile, error) {
	ctx, span := tracer.Start(ctx, "Assistant.GetProfile")
	defer span.End()

	cacheKey := fmt.Sprintf("profile:%s", customerID)
	if cached, ok := a.cache.Get(cacheKey); ok {
		if p, ok := cached.(*domain.CustomerProfile); ok {
			a.metrics.IncrCacheHit("profile")
			return p, nil
		}
	}
	a.metrics.IncrCacheMiss("profile")

	p, err := a.profileClient.GetProfile(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("profile fetch: %w", err)
	}
	a.cache.Set(cacheKey, p)
	return p, nil
}

// GetTransactions fetches the customer transactions (used by the dedicated /transactions route).
func (a *Assistant) GetTransactions(ctx context.Context, customerID string) ([]domain.Transaction, error) {
	ctx, span := tracer.Start(ctx, "Assistant.GetTransactions")
	defer span.End()

	return a.transactionsClient.GetTransactions(ctx, customerID)
}

// GetAssistantResponse orchestrates all external calls and returns the final response.
// It uses concurrent calls for profile and transactions, then calls the AI agent.
func (a *Assistant) GetAssistantResponse(ctx context.Context, customerID string, message string) (*domain.InternalAssistantResult, error) {
	// Bail out early if the caller already cancelled.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracer.Start(ctx, "Assistant.GetAssistantResponse")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", customerID))

	start := time.Now()
	defer func() {
		a.metrics.RecordRequestDuration("assistant", time.Since(start))
	}()

	// --- Step 1: Fetch profile + transactions concurrently ---
	var (
		profile      *domain.CustomerProfile
		transactions []domain.Transaction
	)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		// Check cache first
		cacheKey := fmt.Sprintf("profile:%s", customerID)
		if cached, ok := a.cache.Get(cacheKey); ok {
			if p, ok := cached.(*domain.CustomerProfile); ok {
				profile = p
				a.metrics.IncrCacheHit("profile")
				return nil
			}
		}
		a.metrics.IncrCacheMiss("profile")

		p, err := a.profileClient.GetProfile(gCtx, customerID)
		if err != nil {
			a.logger.Error("failed to fetch profile",
				zap.String("customer_id", customerID),
				zap.Error(err),
			)
			a.metrics.IncrExternalError("profile")
			return fmt.Errorf("profile fetch: %w", err)
		}
		profile = p
		a.cache.Set(cacheKey, p)
		return nil
	})

	g.Go(func() error {
		t, err := a.transactionsClient.GetTransactions(gCtx, customerID)
		if err != nil {
			a.logger.Error("failed to fetch transactions",
				zap.String("customer_id", customerID),
				zap.Error(err),
			)
			a.metrics.IncrExternalError("transactions")
			return fmt.Errorf("transactions fetch: %w", err)
		}
		transactions = t
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// --- Step 2: Call AI Agent ---
	agentReq := &domain.AgentRequest{
		CustomerID:   customerID,
		Profile:      profile,
		Transactions: transactions,
		Query:        message,
	}

	agentStart := time.Now()
	agentResp, err := a.agentClient.Call(ctx, agentReq)
	a.metrics.RecordRequestDuration("agent", time.Since(agentStart))

	if err != nil {
		a.logger.Error("agent call failed",
			zap.String("customer_id", customerID),
			zap.Error(err),
		)
		a.metrics.IncrExternalError("agent")
		return nil, fmt.Errorf("agent call: %w", err)
	}

	// --- Step 3: Record token metrics ---
	a.metrics.RecordTokens(agentResp.TokensUsed.PromptTokens, agentResp.TokensUsed.CompletionTokens)

	return &domain.InternalAssistantResult{
		CustomerID:     customerID,
		Profile:        profile,
		Recommendation: agentResp,
		ProcessedAt:    time.Now(),
	}, nil
}
