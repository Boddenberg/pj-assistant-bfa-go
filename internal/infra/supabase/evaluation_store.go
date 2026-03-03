package supabase

import (
	"context"
	"fmt"
)

// ============================================================
// LLM Evaluations — insert via PostgREST
// ============================================================

// EvaluationRow representa a linha principal da tabela llm_evaluations.
type EvaluationRow struct {
	CustomerID         string  `json:"customer_id"`
	OverallScore       float64 `json:"overall_score"`
	Verdict            string  `json:"verdict"`
	Summary            string  `json:"summary,omitempty"`
	NumTurns           int     `json:"num_turns,omitempty"`
	JudgeModel         string  `json:"judge_model,omitempty"`
	JudgePromptVersion string  `json:"judge_prompt_version,omitempty"`
	TokensUsed         int     `json:"tokens_used,omitempty"`
	EstimatedCostUSD   float64 `json:"estimated_cost_usd,omitempty"`
	EvalDurationMs     float64 `json:"evaluation_duration_ms,omitempty"`
}

// EvaluationCriterionRow representa uma linha da tabela llm_evaluation_criteria.
type EvaluationCriterionRow struct {
	EvaluationID string  `json:"evaluation_id"`
	Criterion    string  `json:"criterion"`
	Score        float64 `json:"score"`
	MaxScore     float64 `json:"max_score"`
	Reasoning    string  `json:"reasoning,omitempty"`
}

// EvaluationImprovementRow representa uma linha da tabela llm_evaluation_improvements.
type EvaluationImprovementRow struct {
	EvaluationID string `json:"evaluation_id"`
	Suggestion   string `json:"suggestion"`
}

// InsertEvaluation insere a avaliação principal e retorna o ID gerado.
func (c *Client) InsertEvaluation(ctx context.Context, row EvaluationRow) (string, error) {
	payload := map[string]any{
		"customer_id":            row.CustomerID,
		"overall_score":          row.OverallScore,
		"verdict":                row.Verdict,
		"summary":                row.Summary,
		"num_turns":              row.NumTurns,
		"judge_model":            row.JudgeModel,
		"judge_prompt_version":   row.JudgePromptVersion,
		"tokens_used":            row.TokensUsed,
		"estimated_cost_usd":     row.EstimatedCostUSD,
		"evaluation_duration_ms": row.EvalDurationMs,
	}

	body, err := c.doPost(ctx, "llm_evaluations", payload)
	if err != nil {
		return "", fmt.Errorf("insert evaluation: %w", err)
	}

	id, err := extractIDFromResponse(body)
	if err != nil {
		return "", fmt.Errorf("insert evaluation: extract id: %w", err)
	}
	return id, nil
}

// InsertEvaluationCriteria insere múltiplos critérios de uma avaliação.
func (c *Client) InsertEvaluationCriteria(ctx context.Context, rows []EvaluationCriterionRow) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := c.doPostAny(ctx, "llm_evaluation_criteria", rows)
	if err != nil {
		return fmt.Errorf("insert evaluation criteria: %w", err)
	}
	return nil
}

// InsertEvaluationImprovements insere múltiplas sugestões de melhoria.
func (c *Client) InsertEvaluationImprovements(ctx context.Context, rows []EvaluationImprovementRow) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := c.doPostAny(ctx, "llm_evaluation_improvements", rows)
	if err != nil {
		return fmt.Errorf("insert evaluation improvements: %w", err)
	}
	return nil
}
