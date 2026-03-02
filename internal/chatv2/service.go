package chatv2

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// Service orquestra o turno de chat:
// frontend → BFA (valida) → Agent Python (dialoga) → frontend.
//
// REGRA DE OURO: O BFA controla deterministicamente o step esperado.
// O agente Python é apenas um formatador de mensagens.
// O BFA NUNCA confia no step/next_step do agente para decidir
// em qual campo está — usa session.ExpectedStep.
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

	// Retomada: se a sessão em memória está vazia, tentar carregar do banco
	if len(session.OnboardingData) == 0 {
		if savedData, err := s.repo.LoadSession(ctx, customerID); err == nil && savedData != nil {
			for k, v := range savedData {
				session.OnboardingData[k] = v
			}
			// Recalcular ExpectedStep com base nos dados já salvos
			for _, step := range OnboardingSequence {
				if _, ok := session.OnboardingData[step]; !ok {
					session.ExpectedStep = step
					break
				}
			}
			s.logger.Info("♻️  sessão retomada do banco",
				zap.String("customer_id", customerID),
				zap.Int("fields_loaded", len(savedData)),
				zap.String("expected_step", session.ExpectedStep),
			)
		}
	}

	// 1. Chamar agente com history atual
	agentResp, err := s.callAgent(ctx, customerID, query, session, "")
	if err != nil {
		return nil, fmt.Errorf("agent call failed: %w", err)
	}

	// 2. Processar resposta do agente com lógica determinística do BFA
	return s.processAgentResponse(ctx, customerID, query, session, agentResp)
}

// callAgent monta o AgentRequest e chama o Agent Python.
func (s *Service) callAgent(ctx context.Context, customerID, query string, session *Session, validationError string) (*AgentResponse, error) {
	req := AgentRequest{
		CustomerID:      customerID,
		Query:           query,
		History:         session.History,
		ValidationError: validationError,
		CollectedData:   session.CollectedData(),
	}

	return s.client.Send(ctx, req)
}

// processAgentResponse aplica as regras DETERMINÍSTICAS do BFA.
//
// Fluxo:
// 1. Se não é onboarding (step vazio) → pass-through.
// 2. Se é welcome → setar ExpectedStep = primeiro campo, pass-through.
// 3. Se é onboarding → BFA usa ExpectedStep para validar query.
//    - Válido → salva, avança ExpectedStep, pede ao agente a próxima mensagem.
//    - Inválido → incrementa retries, pede ao agente mensagem de erro.
func (s *Service) processAgentResponse(ctx context.Context, customerID, query string, session *Session, resp *AgentResponse) (*FrontendResponse, error) {
	agentStep := derefStr(resp.Step)

	// Caso 1: step vazio → conversa normal, não é onboarding
	if agentStep == "" {
		s.appendHistory(session, query, resp.Answer, nil, nil)
		return s.buildResponse(resp, nil), nil
	}

	// Caso 2: welcome → inicializar sequência de onboarding
	if agentStep == "welcome" {
		session.ExpectedStep = OnboardingSequence[0] // cnpj
		session.Retries = 0
		s.appendHistory(session, query, resp.Answer, nil, nil)

		s.logger.Info("🏁 onboarding iniciado",
			zap.String("customer_id", customerID),
			zap.String("expected_step", session.ExpectedStep),
		)
		return s.buildResponse(resp, nil), nil
	}

	// Caso 3: onboarding em andamento → BFA valida usando ExpectedStep
	expected := session.ExpectedStep
	if expected == "" || expected == "completed" {
		// Sessão já completou ou não iniciou — pass-through
		s.appendHistory(session, query, resp.Answer, nil, nil)
		return s.buildResponse(resp, nil), nil
	}

	s.logger.Info("🔍 BFA validando campo",
		zap.String("expected_step", expected),
		zap.String("agent_step", agentStep),
		zap.String("query", query),
	)

	// Sempre validar a QUERY do usuário (não o field_value do agente)
	// O field_value do agente pode estar contaminado com valores de turnos anteriores
	fieldValue := query

	validator, hasValidator := s.validators[expected]
	if !hasValidator {
		s.logger.Warn("no validator for expected step — skipping", zap.String("step", expected))
		session.OnboardingData[expected] = strings.TrimSpace(fieldValue)
		s.appendHistory(session, query, resp.Answer, &expected, boolPtr(true))
		session.AdvanceStep()
		return s.buildResponse(resp, nil), nil
	}

	// Validar
	validationErr := validator.Validate(ctx, fieldValue, session)

	if validationErr != nil {
		// ❌ Validação falhou
		session.Retries++
		remaining := MaxRetries - session.Retries
		if remaining < 0 {
			remaining = 0
		}

		s.logger.Info("❌ BFA rejeitou campo",
			zap.String("step", expected),
			zap.String("field_value", fieldValue),
			zap.Int("retries", session.Retries),
			zap.Int("remaining", remaining),
			zap.Error(validationErr),
		)

		s.appendHistory(session, query, resp.Answer, &expected, boolPtr(false))

		// Chamar agente com validation_error para que formate a mensagem de erro
		errorResp, retryErr := s.callAgent(ctx, customerID, query, session, validationErr.Error())
		if retryErr != nil {
			return nil, fmt.Errorf("agent error-format call failed: %w", retryErr)
		}

		// Corrigir step/next_step na resposta para refletir o step real do BFA
		errorResp.Step = &expected
		errorResp.NextStep = &expected

		return s.buildResponse(errorResp, nil), nil
	}

	// ✅ Validação OK → normalizar, salvar, avançar
	normalized := normalizeFieldValue(expected, fieldValue)
	session.OnboardingData[expected] = normalized

	s.logger.Info("✅ BFA validou campo",
		zap.String("step", expected),
		zap.String("raw", fieldValue),
		zap.String("normalized", normalized),
	)

	// Salvar no banco
	if saveErr := s.repo.SaveField(ctx, customerID, expected, normalized); saveErr != nil {
		s.logger.Error("failed to save field", zap.String("step", expected), zap.Error(saveErr))
	}

	s.appendHistory(session, query, resp.Answer, &expected, boolPtr(true))

	// Avançar para próximo step
	previousStep := expected
	session.AdvanceStep()
	nextExpected := session.ExpectedStep

	s.logger.Info("➡️  avançando step",
		zap.String("completed_step", previousStep),
		zap.String("next_expected_step", nextExpected),
	)

	// Se completed → finalizar conta
	if nextExpected == "completed" {
		missing := session.MissingFields()
		if len(missing) > 0 {
			s.logger.Warn("⚠️ campos faltando apesar de completed",
				zap.Strings("missing_fields", missing),
			)
			// Voltar para o primeiro campo faltante
			session.ExpectedStep = missing[0]
			session.Retries = 0
			validationMsg := fmt.Sprintf("Campos obrigatórios faltando: %s",
				strings.Join(missing, ", "))

			retryResp, retryErr := s.callAgent(ctx, customerID, query, session, validationMsg)
			if retryErr != nil {
				return nil, fmt.Errorf("agent retry missing fields: %w", retryErr)
			}
			return s.buildResponse(retryResp, nil), nil
		}

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

		// Chamar agente para gerar mensagem de parabéns com os dados
		completedResp, completedErr := s.callAgent(ctx, customerID, query, session, "")
		if completedErr != nil {
			// Fallback: usar a resposta original do agente
			return s.buildResponse(resp, accountData), nil
		}
		return s.buildResponse(completedResp, accountData), nil
	}

	// Chamar agente para gerar mensagem de "campo aceito + próximo passo"
	// O agente recebe collected_data atualizado e sabe qual é o próximo
	advanceResp, advanceErr := s.callAgent(ctx, customerID, query, session, "")
	if advanceErr != nil {
		// Fallback: usar resposta original do agente
		return s.buildResponse(resp, nil), nil
	}

	// Corrigir step/next_step para refletir o estado real do BFA
	advanceResp.Step = &previousStep
	advanceResp.NextStep = &nextExpected

	return s.buildResponse(advanceResp, nil), nil
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
