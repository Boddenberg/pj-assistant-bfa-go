package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

/*
 * Chat Metrics — RPC call to get_chat_metrics()
 */

// ChatMetricsRow é o resultado retornado pela função RPC get_chat_metrics().
// Agrupado por seções: agent_performance, rag_quality, llm_judge.
type ChatMetricsRow struct {
	AgentPerformance AgentPerformanceRow `json:"agent_performance"`
	RagQuality       RagQualityRow       `json:"rag_quality"`
	LLMJudge         LLMJudgeRow         `json:"llm_judge"`
}

type AgentPerformanceRow struct {
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	P95LatencyMs        float64 `json:"p95_latency_ms"`
	AvgBfaLatencyMs     float64 `json:"avg_bfa_latency_ms"`
	P95BfaLatencyMs     float64 `json:"p95_bfa_latency_ms"`
	AvgTokensPerRequest float64 `json:"avg_tokens_per_request"`
	TotalTokens         int64   `json:"total_tokens"`
	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`
	TotalRequests       int64   `json:"total_requests"`
	ErrorRatePct        float64 `json:"error_rate_pct"`
	ErrorCount          int64   `json:"error_count"`
	SuccessCount        int64   `json:"success_count"`
	CacheHitRatePct     float64 `json:"cache_hit_rate_pct"`
}

type RagQualityRow struct {
	ScorePct            float64 `json:"score_pct"`
	AvgFaithfulness     float64 `json:"avg_faithfulness"`
	AvgContextRelevance float64 `json:"avg_context_relevance"`
}

type LLMJudgeRow struct {
	TotalEvaluations        int64                `json:"total_evaluations"`
	AvgOverallScore         float64              `json:"avg_overall_score"`
	PassRatePct             float64              `json:"pass_rate_pct"`
	PassCount               int64                `json:"pass_count"`
	FailCount               int64                `json:"fail_count"`
	WarningCount            int64                `json:"warning_count"`
	AvgTurnsPerConversation float64              `json:"avg_turns_per_conversation"`
	AvgEvalDurationMs       float64              `json:"avg_eval_duration_ms"`
	TotalEvalCostUSD        float64              `json:"total_eval_cost_usd"`
	CriteriaBreakdown       []CriterionStatRow   `json:"criteria_breakdown"`
	TopImprovements         []ImprovementStatRow `json:"top_improvements"`
}

type CriterionStatRow struct {
	Criterion string  `json:"criterion"`
	AvgScore  float64 `json:"avg_score"`
	MaxScore  float64 `json:"max_score"`
	AvgPct    float64 `json:"avg_pct"`
}

type ImprovementStatRow struct {
	Suggestion string `json:"suggestion"`
	Count      int    `json:"count"`
}

// GetChatMetrics chama a função RPC get_chat_metrics() no Supabase.
func (c *Client) GetChatMetrics(ctx context.Context) (*ChatMetricsRow, error) {
	// PostgREST RPC: POST /rest/v1/rpc/get_chat_metrics
	body, err := c.doRPC(ctx, "get_chat_metrics")
	if err != nil {
		return nil, fmt.Errorf("get chat metrics: %w", err)
	}

	var row ChatMetricsRow
	if err := json.Unmarshal(body, &row); err != nil {
		return nil, fmt.Errorf("decode chat metrics: %w", err)
	}
	return &row, nil
}

// doRPC chama uma função PostgreSQL via PostgREST RPC (POST /rest/v1/rpc/{function}).
func (c *Client) doRPC(ctx context.Context, functionName string) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/rpc/%s", c.baseURL, functionName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceRoleKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc %s: request failed: %w", functionName, err)
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rpc %s returned %d: %s", functionName, resp.StatusCode, string(body))
	}

	return body, nil
}
