package chatv2

import (
	"context"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/supabase"
	"go.uber.org/zap"
)

// ============================================================
// EvaluationRepository — persiste resultado do LLM-as-Judge
// ============================================================

// EvaluationRepository persiste avaliações do LLM-as-Judge.
type EvaluationRepository interface {
	// SaveEvaluation persiste a avaliação completa (score, critérios, melhorias).
	SaveEvaluation(ctx context.Context, eval EvaluateResponse) error
}

// --- Supabase implementation ---

type SupabaseEvaluationRepository struct {
	sb     *supabase.Client
	logger *zap.Logger
}

func NewSupabaseEvaluationRepository(sb *supabase.Client, logger *zap.Logger) *SupabaseEvaluationRepository {
	return &SupabaseEvaluationRepository{sb: sb, logger: logger}
}

func (r *SupabaseEvaluationRepository) SaveEvaluation(ctx context.Context, eval EvaluateResponse) error {
	// 1. Insere a avaliação principal e obtém o ID
	evalID, err := r.sb.InsertEvaluation(ctx, supabase.EvaluationRow{
		CustomerID:         eval.CustomerID,
		OverallScore:       eval.OverallScore,
		Verdict:            eval.Verdict,
		Summary:            eval.Summary,
		NumTurns:           eval.NumTurns,
		JudgeModel:         eval.Metadata.JudgeModel,
		JudgePromptVersion: eval.Metadata.JudgePromptVersion,
		TokensUsed:         eval.Metadata.TokensUsed,
		EstimatedCostUSD:   eval.Metadata.EstimatedCostUSD,
		EvalDurationMs:     eval.Metadata.EvalDurationMs,
	})
	if err != nil {
		return err
	}

	r.logger.Info("📊 avaliação salva",
		zap.String("evaluation_id", evalID),
		zap.String("customer_id", eval.CustomerID),
		zap.Float64("overall_score", eval.OverallScore),
		zap.String("verdict", eval.Verdict),
	)

	// 2. Insere os critérios
	if len(eval.Criteria) > 0 {
		criteriaRows := make([]supabase.EvaluationCriterionRow, len(eval.Criteria))
		for i, c := range eval.Criteria {
			criteriaRows[i] = supabase.EvaluationCriterionRow{
				EvaluationID: evalID,
				Criterion:    c.Criterion,
				Score:        c.Score,
				MaxScore:     c.MaxScore,
				Reasoning:    c.Reasoning,
			}
		}
		if err := r.sb.InsertEvaluationCriteria(ctx, criteriaRows); err != nil {
			r.logger.Warn("failed to save evaluation criteria",
				zap.String("evaluation_id", evalID),
				zap.Error(err),
			)
			return err
		}
	}

	// 3. Insere as sugestões de melhoria
	if len(eval.Improvements) > 0 {
		improvRows := make([]supabase.EvaluationImprovementRow, len(eval.Improvements))
		for i, suggestion := range eval.Improvements {
			improvRows[i] = supabase.EvaluationImprovementRow{
				EvaluationID: evalID,
				Suggestion:   suggestion,
			}
		}
		if err := r.sb.InsertEvaluationImprovements(ctx, improvRows); err != nil {
			r.logger.Warn("failed to save evaluation improvements",
				zap.String("evaluation_id", evalID),
				zap.Error(err),
			)
			return err
		}
	}

	return nil
}

// --- In-memory stub (para testes) ---

type InMemoryEvaluationRepository struct {
	Evaluations []EvaluateResponse
	logger      *zap.Logger
}

func NewInMemoryEvaluationRepository(logger *zap.Logger) *InMemoryEvaluationRepository {
	return &InMemoryEvaluationRepository{logger: logger}
}

func (r *InMemoryEvaluationRepository) SaveEvaluation(_ context.Context, eval EvaluateResponse) error {
	r.Evaluations = append(r.Evaluations, eval)
	r.logger.Debug("stub: evaluation saved",
		zap.String("customer_id", eval.CustomerID),
		zap.Float64("overall_score", eval.OverallScore),
		zap.String("verdict", eval.Verdict),
	)
	return nil
}
