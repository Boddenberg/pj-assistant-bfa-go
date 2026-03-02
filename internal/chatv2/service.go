package chatv2

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// Service orquestra o turno de chat:
// frontend → BFA (valida) → Agent Python (dialoga) → frontend.
type Service struct {
	client     *Client
	sessions   *SessionStore
	validators ValidatorRegistry
	repo       AccountRepository
	logger     *zap.Logger
}

func NewService(client *Client, sessions *SessionStore, repo AccountRepository, logger *zap.Logger) *Service {
	return &Service{
		client:     client,
		sessions:   sessions,
		validators: NewValidatorRegistry(repo),
		repo:       repo,
		logger:     logger,
	}
}

// ProcessTurn executa um turno completo da conversa.
func (s *Service) ProcessTurn(ctx context.Context, customerID, query string) (*FrontendResponse, error) {
	if customerID == "" {
		customerID = "anonymous"
	}

	session := s.sessions.Get(customerID)

	// 1. Chamar agente com history atual (sem validation_error na primeira tentativa)
	agentResp, err := s.callAgent(ctx, customerID, query, session.History, "")
	if err != nil {
		return nil, fmt.Errorf("agent call failed: %w", err)
	}

	// 2. Processar resposta do agente
	return s.processAgentResponse(ctx, customerID, query, session, agentResp)
}

// callAgent monta o AgentRequest e chama o Agent Python.
func (s *Service) callAgent(ctx context.Context, customerID, query string, history []ChatMessage, validationError string) (*AgentResponse, error) {
	req := AgentRequest{
		CustomerID:      customerID,
		Query:           query,
		History:         history,
		ValidationError: validationError,
	}

	return s.client.Send(ctx, req)
}

// processAgentResponse aplica as regras de processamento do turno.
func (s *Service) processAgentResponse(ctx context.Context, customerID, query string, session *Session, resp *AgentResponse) (*FrontendResponse, error) {
	step := derefStr(resp.Step)
	nextStep := derefStr(resp.NextStep)

	// Caso 1: step == null → conversa normal, não é onboarding
	if step == "" {
		s.appendHistory(session, query, resp.Answer, nil, nil)
		return s.buildResponse(resp, nil), nil
	}

	// Caso 2: step == "welcome" → mensagem de boas-vindas (sem validação)
	if step == "welcome" {
		s.appendHistory(session, query, resp.Answer, nil, nil)
		return s.buildResponse(resp, nil), nil
	}

	// Caso 3: step == next_step → rejeição inline do agente (não avançou)
	// Porém, o BFA tenta validar por conta própria — se passar, reenvia ao agente
	// com o valor validado para forçar o avanço.
	if step == nextStep {
		s.logger.Info("agent inline rejection, BFA will attempt own validation", zap.String("step", step))

		fieldValue := derefStr(resp.FieldValue)
		if fieldValue == "" {
			fieldValue = query // agente pode não ter extraído, usa query original
		}

		if validator, ok := s.validators[step]; ok {
			if err := validator.Validate(ctx, fieldValue, session); err == nil {
				// BFA validou com sucesso — normaliza, salva e reenvia ao agente para avançar
				normalized := normalizeFieldValue(step, fieldValue)
				session.OnboardingData[step] = normalized
				s.logger.Info("BFA overrode agent rejection — field is valid",
					zap.String("step", step),
					zap.String("raw", fieldValue),
					zap.String("normalized", normalized),
				)

				// Salvar campo no banco
				if saveErr := s.repo.SaveField(ctx, customerID, step, normalized); saveErr != nil {
					s.logger.Error("failed to save field", zap.String("step", step), zap.Error(saveErr))
				}

				s.appendHistory(session, query, resp.Answer, &step, boolPtr(true))

				retryResp, retryErr := s.callAgent(ctx, customerID, query, session.History, "")
				if retryErr != nil {
					return nil, fmt.Errorf("agent retry after BFA validation: %w", retryErr)
				}
				return s.buildResponse(retryResp, nil), nil
			}
		}

		// BFA também rejeitou (ou não tem validator) — mantém rejeição do agente
		s.appendHistory(session, query, resp.Answer, &step, boolPtr(false))
		return s.buildResponse(resp, nil), nil
	}

	// Caso 4: campo de onboarding → validar field_value
	fieldValue := derefStr(resp.FieldValue)
	if fieldValue == "" {
		fieldValue = query // fallback: agente não extraiu, usa a query original
	}
	validator, hasValidator := s.validators[step]

	if !hasValidator {
		// Step desconhecido, salva sem validação
		s.logger.Warn("no validator for step", zap.String("step", step))
		s.appendHistory(session, query, resp.Answer, &step, boolPtr(true))
		return s.buildResponse(resp, nil), nil
	}

	// Validar
	if err := validator.Validate(ctx, fieldValue, session); err != nil {
		// Validação falhou → salva validated=false e reenvia com validation_error
		s.logger.Info("validation failed",
			zap.String("step", step),
			zap.String("field_value", fieldValue),
			zap.Error(err),
		)
		s.appendHistory(session, query, resp.Answer, &step, boolPtr(false))

		// Retry: chamar agente com validation_error
		retryResp, retryErr := s.callAgent(ctx, customerID, query, session.History, err.Error())
		if retryErr != nil {
			return nil, fmt.Errorf("agent retry call failed: %w", retryErr)
		}

		return s.buildResponse(retryResp, nil), nil
	}

	// Validação OK → normalizar e persistir na sessão
	normalized := normalizeFieldValue(step, fieldValue)
	session.OnboardingData[step] = normalized
	s.logger.Info("field validated and saved",
		zap.String("step", step),
		zap.String("raw", fieldValue),
		zap.String("normalized", normalized),
	)

	// Salvar campo no banco
	if saveErr := s.repo.SaveField(ctx, customerID, step, normalized); saveErr != nil {
		s.logger.Error("failed to save field", zap.String("step", step), zap.Error(saveErr))
	}

	s.appendHistory(session, query, resp.Answer, &step, boolPtr(true))

	// Se next_step == "completed", finalizar cadastro e devolver account_data
	if nextStep == "completed" {
		accountData, err := s.repo.FinalizeAccount(ctx, customerID, session.OnboardingData)
		if err != nil {
			s.logger.Error("finalize account failed", zap.Error(err))
			return nil, fmt.Errorf("finalize account: %w", err)
		}
		s.logger.Info("🎉 onboarding completed",
			zap.String("customer_id", accountData.CustomerID),
			zap.String("agencia", accountData.Agencia),
			zap.String("conta", accountData.Conta),
		)
		return s.buildResponse(resp, accountData), nil
	}

	return s.buildResponse(resp, nil), nil
}

// appendHistory adiciona uma entrada no history da sessão.
func (s *Service) appendHistory(session *Session, query, answer string, step *string, validated *bool) {
	session.History = append(session.History, ChatMessage{
		Query:     query,
		Answer:    answer,
		Step:      step,
		Validated: validated,
	})
}

// buildResponse monta a FrontendResponse a partir da AgentResponse.
func (s *Service) buildResponse(resp *AgentResponse, accountData *AccountData) *FrontendResponse {
	return &FrontendResponse{
		Answer:      resp.Answer,
		Context:     resp.Context,
		Step:        resp.Step,
		NextStep:    resp.NextStep,
		AccountData: accountData,
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// normalizeFieldValue limpa o valor antes de salvar na sessão.
// Campos numéricos (CNPJ, CPF, telefone) → apenas dígitos.
// Demais campos → trim de espaços.
func normalizeFieldValue(step, value string) string {
	switch step {
	case "cnpj", "representanteCpf", "representantePhone":
		return onlyDigits(value)
	default:
		return strings.TrimSpace(value)
	}
}
