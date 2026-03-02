package chatv2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Client chama o Agent Python via HTTP.
type Client struct {
	httpClient *http.Client
	baseURL    string
	logger     *zap.Logger
}

func NewClient(baseURL string, timeout time.Duration, logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    baseURL,
		logger:     logger,
	}
}

// Send envia AgentRequest para POST {baseURL}/v1/chat e retorna AgentResponse.
func (c *Client) Send(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("agent client: marshal: %w", err)
	}

	url := c.baseURL + "/v1/chat"

	c.logger.Info("➡️  request enviada ao agente Python",
		zap.String("url", url),
		zap.String("customer_id", req.CustomerID),
		zap.String("query", req.Query),
		zap.Int("history_len", len(req.History)),
		zap.String("validation_error", req.ValidationError),
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("agent client: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	latency := time.Since(start)

	if err != nil {
		c.logger.Error("❌ agente Python não respondeu",
			zap.Duration("latency", latency),
			zap.Error(err),
		)
		return nil, fmt.Errorf("agent client: request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("agent client: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("⚠️  agente Python retornou erro",
			zap.Int("status", resp.StatusCode),
			zap.Duration("latency", latency),
			zap.String("body", string(rawBody)),
		)
		return nil, fmt.Errorf("agent client: status %d: %s", resp.StatusCode, string(rawBody))
	}

	var agentResp AgentResponse
	if err := json.Unmarshal(rawBody, &agentResp); err != nil {
		return nil, fmt.Errorf("agent client: unmarshal: %w", err)
	}

	c.logger.Info("⬅️  response recebida do agente Python",
		zap.Duration("latency", latency),
		zap.String("answer", truncateStr(agentResp.Answer, 120)),
		zap.Any("step", agentResp.Step),
		zap.Any("field_value", agentResp.FieldValue),
		zap.Any("next_step", agentResp.NextStep),
		zap.Any("context", agentResp.Context),
	)

	return &agentResp, nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
