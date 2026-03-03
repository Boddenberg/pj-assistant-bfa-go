package chat

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
	maxRetries int           // quantas vezes retentar após falha (0 = sem retry)
	retryDelay time.Duration // delay entre retries
	logger     *zap.Logger
}

func NewClient(baseURL string, timeout time.Duration, maxRetries int, retryDelay time.Duration, logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    baseURL,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		logger:     logger,
	}
}

// Send envia AgentRequest para POST {baseURL}/v1/chat e retorna AgentResponse.
// Inclui retry automático com delay configurável em caso de falha de rede (EOF, timeout, etc).
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

	var lastErr error
	attempts := 1 + c.maxRetries // 1 tentativa original + N retries

	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			c.logger.Warn("🔄 retry ao agente Python",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", attempts),
				zap.Duration("delay", c.retryDelay),
				zap.Error(lastErr),
			)
			select {
			case <-time.After(c.retryDelay):
			case <-ctx.Done():
				return nil, fmt.Errorf("agent client: context cancelled during retry: %w", ctx.Err())
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("agent client: new request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		start := time.Now()
		resp, err := c.httpClient.Do(httpReq)
		latency := time.Since(start)

		if err != nil {
			lastErr = err
			c.logger.Error("❌ agente Python não respondeu",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", attempts),
				zap.Duration("latency", latency),
				zap.Error(err),
			)
			continue // retry
		}
		defer resp.Body.Close()

		rawBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue // retry
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(rawBody))
			c.logger.Warn("⚠️  agente Python retornou erro 5xx (retentável)",
				zap.Int("attempt", attempt),
				zap.Int("status", resp.StatusCode),
				zap.Duration("latency", latency),
				zap.String("body", string(rawBody)),
			)
			continue // retry on 5xx
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

		if attempt > 1 {
			c.logger.Info("✅ retry bem-sucedido",
				zap.Int("attempt", attempt),
				zap.Duration("latency", latency),
			)
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

	return nil, fmt.Errorf("agent client: request failed after %d attempts: %w", attempts, lastErr)
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Evaluate envia a conversa completa do cliente ao agente para avaliação via LLM-as-Judge.
// Retorna a resposta estruturada com score, critérios e melhorias.
func (c *Client) Evaluate(ctx context.Context, req EvaluateRequest) (*EvaluateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("evaluate client: marshal: %w", err)
	}

	url := c.baseURL + "/v1/evaluate"

	// Log detalhado do REQUEST — cada turno da conversa
	c.logger.Info("📊 enviando conversa para LLM-as-Judge",
		zap.String("url", url),
		zap.String("customer_id", req.CustomerID),
		zap.Int("conversation_turns", len(req.Conversation)),
	)
	for i, turn := range req.Conversation {
		c.logger.Debug("📊 evaluate request — turno",
			zap.Int("turn", i+1),
			zap.String("query", truncateStr(turn.Query, 200)),
			zap.String("answer", truncateStr(turn.Answer, 200)),
			zap.String("step", turn.Step),
			zap.String("intent", turn.Intent),
			zap.Float64("confidence", turn.Confidence),
			zap.Int64("latency_ms", turn.LatencyMs),
			zap.Int("rag_contexts", len(turn.Contexts)),
			zap.String("created_at", turn.CreatedAt),
		)
	}
	c.logger.Debug("📊 evaluate request — body completo",
		zap.String("body", string(body)),
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("evaluate client: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("evaluate client: request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("evaluate client: read body: %w", err)
	}

	if resp.StatusCode >= 300 {
		c.logger.Warn("⚠️  LLM-as-Judge retornou erro",
			zap.Int("status", resp.StatusCode),
			zap.Duration("latency", latency),
			zap.String("body", string(rawBody)),
		)
		return nil, fmt.Errorf("evaluate client: status %d: %s", resp.StatusCode, string(rawBody))
	}

	var evalResp EvaluateResponse
	if err := json.Unmarshal(rawBody, &evalResp); err != nil {
		return nil, fmt.Errorf("evaluate client: unmarshal: %w", err)
	}

	// Log detalhado do RESPONSE
	c.logger.Info("✅ LLM-as-Judge respondeu",
		zap.Duration("latency", latency),
		zap.Float64("overall_score", evalResp.OverallScore),
		zap.String("verdict", evalResp.Verdict),
		zap.Int("criteria_count", len(evalResp.Criteria)),
		zap.Int("improvements_count", len(evalResp.Improvements)),
		zap.String("summary", truncateStr(evalResp.Summary, 300)),
	)
	for _, crit := range evalResp.Criteria {
		c.logger.Debug("📊 evaluate response — critério",
			zap.String("criterion", crit.Criterion),
			zap.Float64("score", crit.Score),
			zap.Float64("max_score", crit.MaxScore),
			zap.String("reasoning", truncateStr(crit.Reasoning, 200)),
		)
	}
	for i, imp := range evalResp.Improvements {
		c.logger.Debug("📊 evaluate response — melhoria",
			zap.Int("index", i+1),
			zap.String("suggestion", imp),
		)
	}
	c.logger.Debug("📊 evaluate response — body completo",
		zap.String("body", string(rawBody)),
	)

	return &evalResp, nil
}
