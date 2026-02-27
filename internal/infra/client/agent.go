package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel/attribute"
)

// AgentClient calls the AI Agent service (Python/LangGraph).
type AgentClient struct {
	httpClient *http.Client
	baseURL    string
	cb         *gobreaker.CircuitBreaker
	cfg        resilience.Config
}

// NewAgentClient creates a new AgentClient.
func NewAgentClient(httpClient *http.Client, baseURL string, cb *gobreaker.CircuitBreaker, cfg resilience.Config) *AgentClient {
	return &AgentClient{
		httpClient: httpClient,
		baseURL:    baseURL,
		cb:         cb,
		cfg:        cfg,
	}
}

// Call invokes the AI agent with customer context and returns its response.
func (c *AgentClient) Call(ctx context.Context, req *domain.AgentRequest) (*domain.AgentResponse, error) {
	ctx, span := tracer.Start(ctx, "AgentClient.Call")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", req.CustomerID))

	var agentResp domain.AgentResponse

	result, err := c.cb.Execute(func() (any, error) {
		var innerErr error
		innerErr = resilience.RetryWithBackoff(ctx, c.cfg, func() error {
			body, err := json.Marshal(req)
			if err != nil {
				return err
			}

			url := fmt.Sprintf("%s/v1/agent/invoke", c.baseURL)
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return err
			}
			httpReq.Header.Set("Content-Type", "application/json")

			resp, err := c.httpClient.Do(httpReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("agent API returned status %d", resp.StatusCode)
			}

			return json.NewDecoder(resp.Body).Decode(&agentResp)
		})
		if innerErr != nil {
			return nil, innerErr
		}
		return &agentResp, nil
	})

	if err != nil {
		return nil, &domain.ErrExternalService{Service: "agent", Err: err}
	}

	return result.(*domain.AgentResponse), nil
}
