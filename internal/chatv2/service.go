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
// REGRA DE OURO:
//   - O AGENTE decide a ordem dos campos, qual step pedir, e o que dizer ao cliente.
//   - O BFA APENAS valida o field_value de acordo com o step que o agente informa,
//     salva no banco, e faz pass-through da resposta do agente para o frontend.
//   - O BFA NÃO sabe a jornada, NÃO controla a sequência, NÃO sobrescreve step/next_step.
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
			s.logger.Info("♻️  sessão retomada do banco",
				zap.String("customer_id", customerID),
				zap.Int("fields_loaded", len(savedData)),
			)
		}
	}

	// 1. Chamar agente — ele decide o que fazer
	agentResp, err := s.callAgent(ctx, customerID, query, session, "")
	if err != nil {
		return nil, fmt.Errorf("agent call failed: %w", err)
	}

	// 2. Processar resposta do agente — BFA apenas valida o campo
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

// processAgentResponse:
// - O AGENTE informa step, next_step, answer, field_value.
// - O BFA usa o step do agente para saber QUAL validator rodar.
// - Se step == next_step → rejeição inline do agente (BFA tenta validar por conta).
// - Se step != next_step → agente aceitou, BFA valida e salva.
// - Se step vazio / welcome → pass-through.
// - O BFA NUNCA sobrescreve step/next_step — o agente controla a jornada.
func (s *Service) processAgentResponse(ctx context.Context, customerID, query string, session *Session, resp *AgentResponse) (*FrontendResponse, error) {
	step := derefStr(resp.Step)
	nextStep := derefStr(resp.NextStep)

	// Caso 1: step vazio → conversa normal, não é onboarding
	if step == "" {
		s.appendHistory(session, query, resp.Answer, nil, nil)
		return s.buildResponse(resp, nil), nil
	}

	// Caso 2: welcome → boas-vindas do agente, pass-through
	if step == "welcome" {
		s.appendHistory(session, query, resp.Answer, nil, nil)
		s.logger.Info("🏁 onboarding iniciado",
			zap.String("customer_id", customerID),
			zap.String("next_step", nextStep),
		)
		return s.buildResponse(resp, nil), nil
	}

	// Controle de retries: se mudou de step, resetar contador
	if step != session.LastStep {
		session.Retries = 0
		session.LastStep = step
	}

	s.logger.Info("🔍 BFA validando campo",
		zap.String("agent_step", step),
		zap.String("agent_next_step", nextStep),
		zap.String("query", query),
	)

	// Caso 3: step == next_step → rejeição inline do agente
	// O agente já rejeitou, mas o BFA tenta validar por conta própria.
	// Se o BFA também rejeitar → reenvia ao agente com validation_error.
	// Se o BFA aceitar → salva e reenvia ao agente para que ele avance.
	if step == nextStep {
		s.logger.Info("agent inline rejection — BFA will attempt own validation", zap.String("step", step))

		// BFA valida a query do usuário
		if validator, ok := s.validators[step]; ok {
			validationErr := validator.Validate(ctx, query, session)
			if validationErr == nil {
				// BFA aceitou — salvar, e reenviar ao agente para que avance
				normalized := normalizeFieldValue(step, query)
				session.OnboardingData[step] = normalized
				session.Retries = 0

				s.logger.Info("✅ BFA overrode agent rejection — field is valid",
					zap.String("step", step),
					zap.String("raw", query),
					zap.String("normalized", normalized),
				)

				if saveErr := s.repo.SaveField(ctx, customerID, step, normalized); saveErr != nil {
					s.logger.Error("failed to save field", zap.String("step", step), zap.Error(saveErr))
				}

				s.appendHistory(session, query, resp.Answer, &step, boolPtr(true))

				// Reenviar ao agente — agora com o campo salvo em collected_data.
				// IMPORTANTE: enviar query vazia para que o agente NÃO interprete
				// a mesma query como valor do próximo campo.
				retryResp, retryErr := s.callAgent(ctx, customerID, "", session, "")
				if retryErr != nil {
					return nil, fmt.Errorf("agent retry after BFA override: %w", retryErr)
				}
				// Pass-through da resposta do agente — ele decide step/next_step
				return s.buildResponse(retryResp, nil), nil
			}

			// BFA também rejeitou — enviar validation_error ao agente
			session.Retries++
			s.logger.Info("❌ BFA also rejected — sending validation_error to agent",
				zap.String("step", step),
				zap.String("field_value", query),
				zap.Int("retries", session.Retries),
				zap.Error(validationErr),
			)

			s.appendHistory(session, query, resp.Answer, &step, boolPtr(false))

			errorResp, retryErr := s.callAgent(ctx, customerID, query, session, validationErr.Error())
			if retryErr != nil {
				return nil, fmt.Errorf("agent error-format call failed: %w", retryErr)
			}
			// Pass-through — agente formata a mensagem de erro
			return s.buildResponse(errorResp, nil), nil
		}

		// Sem validator para o step — manter rejeição do agente
		s.appendHistory(session, query, resp.Answer, &step, boolPtr(false))
		return s.buildResponse(resp, nil), nil
	}

	// Caso 4: step != next_step → agente aceitou e avançou
	// BFA valida o campo usando o step do agente, salva, e faz pass-through.
	validator, hasValidator := s.validators[step]

	if !hasValidator {
		// Step desconhecido — salva sem validação, pass-through
		s.logger.Warn("no validator for step — accepting", zap.String("step", step))
		session.OnboardingData[step] = strings.TrimSpace(query)
		s.appendHistory(session, query, resp.Answer, &step, boolPtr(true))

		if saveErr := s.repo.SaveField(ctx, customerID, step, strings.TrimSpace(query)); saveErr != nil {
			s.logger.Error("failed to save field", zap.String("step", step), zap.Error(saveErr))
		}

		return s.checkCompletion(ctx, customerID, query, session, resp)
	}

	// Validar a query do usuário usando o step que o AGENTE informou
	if err := validator.Validate(ctx, query, session); err != nil {
		// BFA rejeitou apesar do agente ter aceitado
		session.Retries++
		s.logger.Info("❌ BFA rejected field (agent had accepted)",
			zap.String("step", step),
			zap.String("field_value", query),
			zap.Int("retries", session.Retries),
			zap.Error(err),
		)

		s.appendHistory(session, query, resp.Answer, &step, boolPtr(false))

		// Reenviar ao agente com validation_error para que reformate
		errorResp, retryErr := s.callAgent(ctx, customerID, query, session, err.Error())
		if retryErr != nil {
			return nil, fmt.Errorf("agent retry call failed: %w", retryErr)
		}
		// Pass-through
		return s.buildResponse(errorResp, nil), nil
	}

	// ✅ Validação OK — normalizar, salvar
	normalized := normalizeFieldValue(step, query)
	session.OnboardingData[step] = normalized
	session.Retries = 0

	s.logger.Info("✅ BFA validou campo",
		zap.String("step", step),
		zap.String("raw", query),
		zap.String("normalized", normalized),
	)

	if saveErr := s.repo.SaveField(ctx, customerID, step, normalized); saveErr != nil {
		s.logger.Error("failed to save field", zap.String("step", step), zap.Error(saveErr))
	}

	s.appendHistory(session, query, resp.Answer, &step, boolPtr(true))

	// Verificar se o agente disse que completou
	return s.checkCompletion(ctx, customerID, query, session, resp)
}

// checkCompletion verifica se o agente retornou next_step=completed.
// Se sim e todos os campos estão preenchidos, finaliza a conta.
// Caso contrário, faz pass-through da resposta do agente.
func (s *Service) checkCompletion(ctx context.Context, customerID, query string, session *Session, resp *AgentResponse) (*FrontendResponse, error) {
	nextStep := derefStr(resp.NextStep)

	if nextStep != "completed" {
		// Agente não completou — pass-through
		return s.buildResponse(resp, nil), nil
	}

	// Agente disse completed — verificar se todos os campos obrigatórios estão preenchidos
	missing := session.MissingFields()
	if len(missing) > 0 {
		s.logger.Warn("⚠️ agente disse completed mas faltam campos",
			zap.Strings("missing_fields", missing),
		)
		validationMsg := fmt.Sprintf("Não é possível finalizar. Campos faltando: %s",
			strings.Join(missing, ", "))

		retryResp, retryErr := s.callAgent(ctx, customerID, query, session, validationMsg)
		if retryErr != nil {
			return nil, fmt.Errorf("agent retry missing fields: %w", retryErr)
		}
		return s.buildResponse(retryResp, nil), nil
	}

	// Todos os campos preenchidos — finalizar conta
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

	// Pass-through da resposta do agente + dados da conta
	return s.buildResponse(resp, accountData), nil
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
// NUNCA sobrescreve step/next_step — o agente controla a jornada.
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
