package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

// ============================================================
// Onboarding Sessions — CRUD via PostgREST
// ============================================================

// OnboardingRow representa uma linha da tabela onboarding_sessions.
type OnboardingRow struct {
	ID                     string `json:"id"`
	SessionID              string `json:"session_id"`
	CNPJ                   string `json:"cnpj"`
	RazaoSocial            string `json:"razao_social"`
	NomeFantasia           string `json:"nome_fantasia"`
	Email                  string `json:"email"`
	RepresentanteName      string `json:"representante_name"`
	RepresentanteCPF       string `json:"representante_cpf"`
	RepresentantePhone     string `json:"representante_phone"`
	RepresentanteBirthDate string `json:"representante_birth_date"`
	PasswordHash           string `json:"password_hash"`
	Status                 string `json:"status"`
	CustomerID             string `json:"customer_id"`
}

// stepToColumn mapeia o nome do step (camelCase do onboarding)
// para o nome da coluna no banco (snake_case).
var stepToColumn = map[string]string{
	"cnpj":                   "cnpj",
	"razaoSocial":            "razao_social",
	"nomeFantasia":           "nome_fantasia",
	"email":                  "email",
	"representanteName":      "representante_name",
	"representanteCpf":       "representante_cpf",
	"representantePhone":     "representante_phone",
	"representanteBirthDate": "representante_birth_date",
	"password":               "password_hash",
	"passwordConfirmation":   "", // não salva no banco, só valida
}

// UpsertOnboardingField salva um campo validado na tabela onboarding_sessions.
// Se a sessão não existir, cria; se existir, atualiza o campo.
func (c *Client) UpsertOnboardingField(ctx context.Context, sessionID, step, value string) error {
	ctx, span := tracer.Start(ctx, "Supabase.UpsertOnboardingField")
	defer span.End()

	column, ok := stepToColumn[step]
	if !ok || column == "" {
		// Step sem coluna correspondente (ex: passwordConfirmation) — ignora
		return nil
	}

	// Tentar buscar sessão existente
	existing, err := c.GetOnboardingSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get onboarding session: %w", err)
	}

	if existing == nil {
		// Criar nova sessão com o primeiro campo
		data := map[string]any{
			"session_id": sessionID,
			column:       value,
			"status":     "in_progress",
		}
		_, err := c.doPost(ctx, "onboarding_sessions", data)
		if err != nil {
			return fmt.Errorf("create onboarding session: %w", err)
		}
		c.logger.Info("onboarding session created",
			zap.String("session_id", sessionID),
			zap.String("step", step),
		)
		return nil
	}

	// Atualizar campo existente
	path := fmt.Sprintf("onboarding_sessions?session_id=eq.%s", sessionID)
	data := map[string]any{
		column:       value,
		"updated_at": "now()",
	}
	if err := c.doPatch(ctx, path, data); err != nil {
		return fmt.Errorf("update onboarding field: %w", err)
	}

	c.logger.Info("onboarding field saved",
		zap.String("session_id", sessionID),
		zap.String("step", step),
		zap.String("column", column),
	)
	return nil
}

// GetOnboardingSession busca a sessão pelo session_id.
func (c *Client) GetOnboardingSession(ctx context.Context, sessionID string) (*OnboardingRow, error) {
	ctx, span := tracer.Start(ctx, "Supabase.GetOnboardingSession")
	defer span.End()

	path := fmt.Sprintf("onboarding_sessions?session_id=eq.%s&limit=1", sessionID)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}
	if body == nil || string(body) == "[]" {
		return nil, nil
	}

	var rows []OnboardingRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode onboarding_sessions: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// CompleteOnboardingSession marca a sessão como concluída e salva o customerID.
func (c *Client) CompleteOnboardingSession(ctx context.Context, sessionID, customerID string) error {
	ctx, span := tracer.Start(ctx, "Supabase.CompleteOnboardingSession")
	defer span.End()

	path := fmt.Sprintf("onboarding_sessions?session_id=eq.%s", sessionID)
	data := map[string]any{
		"status":      "completed",
		"customer_id": customerID,
		"updated_at":  "now()",
	}
	return c.doPatch(ctx, path, data)
}

// CNPJExistsInOnboarding verifica se um CNPJ já existe em alguma sessão completa.
func (c *Client) CNPJExistsInOnboarding(ctx context.Context, cnpj string) (bool, error) {
	ctx, span := tracer.Start(ctx, "Supabase.CNPJExistsInOnboarding")
	defer span.End()

	path := fmt.Sprintf("onboarding_sessions?cnpj=eq.%s&status=eq.completed&limit=1", cnpj)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return false, err
	}
	if body == nil || string(body) == "[]" {
		return false, nil
	}
	return true, nil
}

// DeleteOnboardingSession remove a sessão temporária do banco.
func (c *Client) DeleteOnboardingSession(ctx context.Context, sessionID string) error {
	ctx, span := tracer.Start(ctx, "Supabase.DeleteOnboardingSession")
	defer span.End()

	path := fmt.Sprintf("onboarding_sessions?session_id=eq.%s", sessionID)
	return c.doDelete(ctx, path)
}
