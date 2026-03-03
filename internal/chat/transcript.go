package chat

import (
	"context"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/supabase"
	"go.uber.org/zap"
)

/*
 * Transcript — registro de cada turno para LLM-as-Judge
 */

// Transcript representa um turno completo da conversa para avaliação.
type Transcript struct {
	CustomerID    string   `json:"customer_id"`
	Query         string   `json:"query"`
	Answer        string   `json:"answer"`
	RagContexts   []string `json:"rag_contexts,omitempty"`
	Step          string   `json:"step,omitempty"`
	Intent        string   `json:"intent,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
	Model         string   `json:"model,omitempty"`
	LatencyMs     int64    `json:"latency_ms,omitempty"`
	ErrorOccurred bool     `json:"error_occurred"`
	TokensUsed    int      `json:"tokens_used,omitempty"`
	BfaLatencyMs  int64    `json:"bfa_latency_ms,omitempty"`
	CreatedAt     string   `json:"created_at,omitempty"`
}

// TranscriptRepository persiste transcrições para avaliação posterior.
type TranscriptRepository interface {
	SaveTranscript(ctx context.Context, t Transcript) error
	// ListTranscripts retorna turnos NÃO avaliados de um cliente, ordenados por created_at.
	ListTranscripts(ctx context.Context, customerID string) ([]Transcript, error)
	// MarkTranscriptsEvaluated marca todas as transcrições de um cliente como avaliadas.
	MarkTranscriptsEvaluated(ctx context.Context, customerID string) error
}

/* Supabase implementation */

type SupabaseTranscriptRepository struct {
	sb     *supabase.Client
	logger *zap.Logger
}

func NewSupabaseTranscriptRepository(sb *supabase.Client, logger *zap.Logger) *SupabaseTranscriptRepository {
	return &SupabaseTranscriptRepository{sb: sb, logger: logger}
}

func (r *SupabaseTranscriptRepository) SaveTranscript(ctx context.Context, t Transcript) error {
	row := map[string]any{
		"customer_id":    t.CustomerID,
		"query":          t.Query,
		"answer":         t.Answer,
		"error_occurred": t.ErrorOccurred,
		"tokens_used":    t.TokensUsed,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
	}
	if t.Step != "" {
		row["step"] = t.Step
	}
	if t.Intent != "" {
		row["intent"] = t.Intent
	}
	if t.Confidence > 0 {
		row["confidence"] = t.Confidence
	}
	if t.Model != "" {
		row["model"] = t.Model
	}
	if t.LatencyMs > 0 {
		row["latency_ms"] = t.LatencyMs
	}
	if len(t.RagContexts) > 0 {
		row["rag_contexts"] = t.RagContexts
	}
	if t.BfaLatencyMs > 0 {
		row["bfa_latency_ms"] = t.BfaLatencyMs
	}

	return r.sb.InsertTranscript(ctx, row)
}

func (r *SupabaseTranscriptRepository) ListTranscripts(ctx context.Context, customerID string) ([]Transcript, error) {
	rows, err := r.sb.ListTranscripts(ctx, customerID)
	if err != nil {
		return nil, err
	}

	transcripts := make([]Transcript, len(rows))
	for i, row := range rows {
		transcripts[i] = Transcript{
			CustomerID:    row.CustomerID,
			Query:         row.Query,
			Answer:        row.Answer,
			RagContexts:   row.RagContexts,
			Step:          row.Step,
			Intent:        row.Intent,
			Confidence:    row.Confidence,
			Model:         row.Model,
			LatencyMs:     row.LatencyMs,
			ErrorOccurred: row.ErrorOccurred,
			TokensUsed:    row.TokensUsed,
			CreatedAt:     row.CreatedAt,
		}
	}
	return transcripts, nil
}

func (r *SupabaseTranscriptRepository) MarkTranscriptsEvaluated(ctx context.Context, customerID string) error {
	return r.sb.MarkTranscriptsEvaluated(ctx, customerID)
}

/* In-memory stub (para testes) */

type InMemoryTranscriptRepository struct {
	Entries []Transcript
	logger  *zap.Logger
}

func NewInMemoryTranscriptRepository(logger *zap.Logger) *InMemoryTranscriptRepository {
	return &InMemoryTranscriptRepository{logger: logger}
}

func (r *InMemoryTranscriptRepository) MarkTranscriptsEvaluated(_ context.Context, customerID string) error {
	r.logger.Debug("stub: transcripts marked as evaluated",
		zap.String("customer_id", customerID),
	)
	return nil
}

func (r *InMemoryTranscriptRepository) SaveTranscript(_ context.Context, t Transcript) error {
	r.Entries = append(r.Entries, t)
	r.logger.Debug("stub: transcript saved",
		zap.String("customer_id", t.CustomerID),
		zap.String("query", t.Query),
	)
	return nil
}

func (r *InMemoryTranscriptRepository) ListTranscripts(_ context.Context, customerID string) ([]Transcript, error) {
	var result []Transcript
	for _, t := range r.Entries {
		if t.CustomerID == customerID {
			result = append(result, t)
		}
	}
	return result, nil
}
