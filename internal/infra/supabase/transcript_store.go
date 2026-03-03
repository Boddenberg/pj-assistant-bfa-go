package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ============================================================
// LLM Transcripts — insert + list via PostgREST
// ============================================================

// TranscriptRow representa uma linha da tabela llm_transcripts.
type TranscriptRow struct {
	ID            string   `json:"id"`
	CustomerID    string   `json:"customer_id"`
	Query         string   `json:"query"`
	Answer        string   `json:"answer"`
	RagContexts   []string `json:"rag_contexts"`
	Step          string   `json:"step"`
	Intent        string   `json:"intent"`
	Confidence    float64  `json:"confidence"`
	Model         string   `json:"model"`
	LatencyMs     int64    `json:"latency_ms"`
	ErrorOccurred bool     `json:"error_occurred"`
	TokensUsed    int      `json:"tokens_used"`
	CreatedAt     string   `json:"created_at"`
}

// InsertTranscript insere um registro na tabela llm_transcripts.
func (c *Client) InsertTranscript(ctx context.Context, row map[string]any) error {
	_, err := c.doPost(ctx, "llm_transcripts", row)
	if err != nil {
		return fmt.Errorf("insert transcript: %w", err)
	}
	return nil
}

// ListTranscripts retorna turnos NÃO avaliados de um cliente, ordenados por created_at asc.
// Usado pelo LLM-as-Judge para avaliar a conversa completa.
// Transcrições já avaliadas (evaluated=true) são ignoradas.
func (c *Client) ListTranscripts(ctx context.Context, customerID string) ([]TranscriptRow, error) {
	path := fmt.Sprintf("llm_transcripts?customer_id=eq.%s&evaluated=eq.false&order=created_at.asc", customerID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, fmt.Errorf("list transcripts: %w", err)
	}

	var rows []TranscriptRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode transcripts: %w", err)
	}
	return rows, nil
}

// MarkTranscriptsEvaluated marca todas as transcrições de um cliente como avaliadas.
// Isso evita que sejam reenviadas para o LLM-as-Judge.
func (c *Client) MarkTranscriptsEvaluated(ctx context.Context, customerID string) error {
	path := fmt.Sprintf("llm_transcripts?customer_id=eq.%s&evaluated=eq.false", customerID)
	return c.doPatch(ctx, path, map[string]any{"evaluated": true})
}
