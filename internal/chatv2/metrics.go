package chatv2

import (
	"context"
	"fmt"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/supabase"
	"go.uber.org/zap"
)

// ============================================================
// ChatMetrics — métricas agregadas do chat para o frontend
// Agrupadas por contexto (seções) — o frontend só exibe.
// ============================================================

// ChatMetricsResponse é o JSON retornado por GET /v1/chat/metrics.
// Agrupado em 3 seções: agent_performance, rag_quality, llm_judge.
type ChatMetricsResponse struct {
	AgentPerformance AgentPerformanceMetrics `json:"agent_performance"`
	RagQuality       RagQualityMetrics       `json:"rag_quality"`
	LLMJudge         LLMJudgeMetrics         `json:"llm_judge"`
}

// AgentPerformanceMetrics — Seção "Desempenho do Agente"
type AgentPerformanceMetrics struct {
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	P95LatencyMs        float64 `json:"p95_latency_ms"`
	AvgTokensPerRequest float64 `json:"avg_tokens_per_request"`
	TotalTokens         int64   `json:"total_tokens"`
	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`
	TotalRequests       int64   `json:"total_requests"`
	ErrorRatePct        float64 `json:"error_rate_pct"`
	ErrorCount          int64   `json:"error_count"`
	SuccessCount        int64   `json:"success_count"`
	CacheHitRatePct     float64 `json:"cache_hit_rate_pct"`
}

// RagQualityMetrics — Seção "Qualidade RAG"
type RagQualityMetrics struct {
	ScorePct            float64 `json:"score_pct"`
	Label               string  `json:"label"`
	AvgFaithfulness     float64 `json:"avg_faithfulness"`
	AvgContextRelevance float64 `json:"avg_context_relevance"`
}

// LLMJudgeMetrics — Seção "LLM-as-Judge"
type LLMJudgeMetrics struct {
	TotalEvaluations        int64             `json:"total_evaluations"`
	AvgOverallScore         float64           `json:"avg_overall_score"`
	PassRatePct             float64           `json:"pass_rate_pct"`
	PassCount               int64             `json:"pass_count"`
	FailCount               int64             `json:"fail_count"`
	WarningCount            int64             `json:"warning_count"`
	AvgTurnsPerConversation float64           `json:"avg_turns_per_conversation"`
	AvgEvalDurationMs       float64           `json:"avg_eval_duration_ms"`
	TotalEvalCostUSD        float64           `json:"total_eval_cost_usd"`
	CriteriaBreakdown       []CriterionStat   `json:"criteria_breakdown"`
	TopImprovements         []ImprovementStat `json:"top_improvements"`
}

// CriterionStat — Score médio de um critério individual.
type CriterionStat struct {
	Criterion string  `json:"criterion"`
	AvgScore  float64 `json:"avg_score"`
	MaxScore  float64 `json:"max_score"`
	AvgPct    float64 `json:"avg_pct"`
}

// ImprovementStat — Sugestão de melhoria e quantas vezes apareceu.
type ImprovementStat struct {
	Suggestion string `json:"suggestion"`
	Count      int    `json:"count"`
}

// MetricsRepository busca métricas agregadas do chat.
type MetricsRepository interface {
	GetChatMetrics(ctx context.Context) (*ChatMetricsResponse, error)
}

// --- Supabase implementation ---

type SupabaseMetricsRepository struct {
	sb     *supabase.Client
	logger *zap.Logger
}

func NewSupabaseMetricsRepository(sb *supabase.Client, logger *zap.Logger) *SupabaseMetricsRepository {
	return &SupabaseMetricsRepository{sb: sb, logger: logger}
}

func (r *SupabaseMetricsRepository) GetChatMetrics(ctx context.Context) (*ChatMetricsResponse, error) {
	row, err := r.sb.GetChatMetrics(ctx)
	if err != nil {
		return nil, err
	}

	ragLabel := "RAG indisponível"
	if row.RagQuality.ScorePct > 0 {
		ragLabel = formatRagLabel(row.RagQuality.ScorePct)
	}

	// Mapear criteria breakdown
	criteria := make([]CriterionStat, len(row.LLMJudge.CriteriaBreakdown))
	for i, c := range row.LLMJudge.CriteriaBreakdown {
		criteria[i] = CriterionStat{
			Criterion: c.Criterion,
			AvgScore:  c.AvgScore,
			MaxScore:  c.MaxScore,
			AvgPct:    c.AvgPct,
		}
	}

	// Mapear top improvements
	improvements := make([]ImprovementStat, len(row.LLMJudge.TopImprovements))
	for i, imp := range row.LLMJudge.TopImprovements {
		improvements[i] = ImprovementStat{
			Suggestion: imp.Suggestion,
			Count:      imp.Count,
		}
	}

	return &ChatMetricsResponse{
		AgentPerformance: AgentPerformanceMetrics{
			AvgLatencyMs:        row.AgentPerformance.AvgLatencyMs,
			P95LatencyMs:        row.AgentPerformance.P95LatencyMs,
			AvgTokensPerRequest: row.AgentPerformance.AvgTokensPerRequest,
			TotalTokens:         row.AgentPerformance.TotalTokens,
			EstimatedCostUSD:    row.AgentPerformance.EstimatedCostUSD,
			TotalRequests:       row.AgentPerformance.TotalRequests,
			ErrorRatePct:        row.AgentPerformance.ErrorRatePct,
			ErrorCount:          row.AgentPerformance.ErrorCount,
			SuccessCount:        row.AgentPerformance.SuccessCount,
			CacheHitRatePct:     row.AgentPerformance.CacheHitRatePct,
		},
		RagQuality: RagQualityMetrics{
			ScorePct:            row.RagQuality.ScorePct,
			Label:               ragLabel,
			AvgFaithfulness:     row.RagQuality.AvgFaithfulness,
			AvgContextRelevance: row.RagQuality.AvgContextRelevance,
		},
		LLMJudge: LLMJudgeMetrics{
			TotalEvaluations:        row.LLMJudge.TotalEvaluations,
			AvgOverallScore:         row.LLMJudge.AvgOverallScore,
			PassRatePct:             row.LLMJudge.PassRatePct,
			PassCount:               row.LLMJudge.PassCount,
			FailCount:               row.LLMJudge.FailCount,
			WarningCount:            row.LLMJudge.WarningCount,
			AvgTurnsPerConversation: row.LLMJudge.AvgTurnsPerConversation,
			AvgEvalDurationMs:       row.LLMJudge.AvgEvalDurationMs,
			TotalEvalCostUSD:        row.LLMJudge.TotalEvalCostUSD,
			CriteriaBreakdown:       criteria,
			TopImprovements:         improvements,
		},
	}, nil
}

// --- In-memory stub (para testes) ---

type InMemoryMetricsRepository struct {
	logger *zap.Logger
}

func NewInMemoryMetricsRepository(logger *zap.Logger) *InMemoryMetricsRepository {
	return &InMemoryMetricsRepository{logger: logger}
}

func (r *InMemoryMetricsRepository) GetChatMetrics(_ context.Context) (*ChatMetricsResponse, error) {
	return &ChatMetricsResponse{
		AgentPerformance: AgentPerformanceMetrics{},
		RagQuality: RagQualityMetrics{
			Label: "RAG indisponível",
		},
		LLMJudge: LLMJudgeMetrics{
			CriteriaBreakdown: []CriterionStat{},
			TopImprovements:   []ImprovementStat{},
		},
	}, nil
}

// formatRagLabel gera o label legível para o score RAG.
// Ex: 85.0 → "RAG 85.0% confiável — Bom"
func formatRagLabel(scorePct float64) string {
	if scorePct >= 90 {
		return fmt.Sprintf("RAG %.1f%% confiável — Excelente", scorePct)
	}
	if scorePct >= 70 {
		return fmt.Sprintf("RAG %.1f%% confiável — Bom", scorePct)
	}
	if scorePct >= 50 {
		return fmt.Sprintf("RAG %.1f%% confiável — Regular", scorePct)
	}
	return fmt.Sprintf("RAG %.1f%% confiável — Precisa melhorar", scorePct)
}
