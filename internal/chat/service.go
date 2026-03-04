package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/port"
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
	client      *Client
	sessions    *SessionStore
	validators  ValidatorRegistry
	repo        AccountRepository
	transcripts TranscriptRepository
	evaluations EvaluationRepository
	ctxFetcher  ContextFetcher // dados financeiros (pode ser nil)
	authStore   port.AuthStore // perfil do cliente (pode ser nil)
	logger      *zap.Logger
}

func NewService(client *Client, sessions *SessionStore, repo AccountRepository, transcripts TranscriptRepository, evaluations EvaluationRepository, ctxFetcher ContextFetcher, authStore port.AuthStore, logger *zap.Logger) *Service {
	return &Service{
		client:      client,
		sessions:    sessions,
		validators:  NewValidatorRegistry(repo),
		repo:        repo,
		transcripts: transcripts,
		evaluations: evaluations,
		ctxFetcher:  ctxFetcher,
		authStore:   authStore,
		logger:      logger,
	}
}

// ProcessTurn executa um turno completo da conversa.
//
// Fluxo simples: busca TODOS os dados financeiros e manda tudo pro agente em uma única chamada.
// Para anônimos: chamada sem contexto financeiro.
func (s *Service) ProcessTurn(ctx context.Context, customerID, query string, isAuthenticated bool) (*FrontendResponse, error) {
	bfaStart := time.Now() // ⏱ medir latência total do BFA

	s.logger.Info("📥 ProcessTurn — entrada",
		zap.String("customer_id", customerID),
		zap.String("query", query),
		zap.Bool("is_authenticated", isAuthenticated),
	)

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

	// ── Buscar contexto financeiro completo (se autenticado) ──
	var financialCtx *FinancialContext
	if customerID != "anonymous" && s.ctxFetcher != nil {
		financialCtx = BuildFinancialContext(ctx, s.ctxFetcher, s.authStore, customerID, s.logger)
	}

	// ── Chamada única ao agente — com tudo ──
	start := time.Now()

	s.logger.Info("📤 chamando agente Python",
		zap.String("customer_id", customerID),
		zap.String("query", query),
		zap.Bool("is_authenticated", isAuthenticated),
		zap.Bool("has_financial_ctx", financialCtx != nil),
		zap.Int("history_len", len(session.History)),
		zap.Int("collected_data_len", len(session.OnboardingData)),
	)

	agentResp, err := s.callAgent(ctx, customerID, query, session, "", financialCtx, isAuthenticated)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		bfaLatencyMs := time.Since(bfaStart).Milliseconds()
		s.saveTranscriptAsync(customerID, query, &AgentResponse{Answer: err.Error()}, latencyMs, bfaLatencyMs, true, nil)

		s.logger.Warn("⚠️  fallback ativado — agente indisponível",
			zap.String("customer_id", customerID),
			zap.Int64("latency_ms", latencyMs),
			zap.Error(err),
		)

		return &FrontendResponse{
			Answer:  "Parece que tivemos uma lentidão no momento. Você poderia repetir sua pergunta?",
			Context: "",
		}, nil
	}

	// 2. Processar resposta do agente — BFA apenas valida o campo
	s.logger.Info("📨 resposta do agente recebida — processando",
		zap.String("customer_id", customerID),
		zap.String("answer", truncateStr(agentResp.Answer, 150)),
		zap.Any("step", agentResp.Step),
		zap.Any("next_step", agentResp.NextStep),
		zap.String("context", agentResp.Context),
		zap.Any("field_value", agentResp.FieldValue),
		zap.Float64("confidence", agentResp.Confidence),
	)
	frontResp, procErr := s.processAgentResponse(ctx, customerID, query, session, agentResp, financialCtx, isAuthenticated)

	// Salvar transcrição de forma assíncrona (LLM-as-Judge) — mede latência total do BFA
	bfaLatencyMs := time.Since(bfaStart).Milliseconds()
	s.saveTranscriptAsync(customerID, query, agentResp, latencyMs, bfaLatencyMs, false, financialCtx)

	return frontResp, procErr
}

// callAgent monta o AgentRequest e chama o Agent Python.
func (s *Service) callAgent(ctx context.Context, customerID, query string, session *Session, validationError string, financialCtx *FinancialContext, isAuthenticated bool) (*AgentResponse, error) {
	req := AgentRequest{
		CustomerID:       customerID,
		Query:            query,
		History:          session.History,
		ValidationError:  validationError,
		CollectedData:    session.CollectedData(),
		IsAuthenticated:  isAuthenticated,
		FinancialContext: financialCtx,
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
func (s *Service) processAgentResponse(ctx context.Context, customerID, query string, session *Session, resp *AgentResponse, financialCtx *FinancialContext, isAuthenticated bool) (*FrontendResponse, error) {
	step := derefStr(resp.Step)
	nextStep := derefStr(resp.NextStep)

	s.logger.Info("🧠 processAgentResponse — entrada",
		zap.String("customer_id", customerID),
		zap.String("query", query),
		zap.Bool("is_authenticated", isAuthenticated),
		zap.String("step", step),
		zap.String("next_step", nextStep),
		zap.String("context", resp.Context),
		zap.String("answer", truncateStr(resp.Answer, 100)),
		zap.Any("field_value", resp.FieldValue),
	)

	// Caso 1: step vazio → conversa normal, não é onboarding
	// Mesmo que context == "onboarding", se step está vazio o agente está
	// apenas respondendo uma pergunta genérica — pass-through.
	if step == "" {
		s.logger.Info("➡️  step vazio — pass-through conversa normal",
			zap.String("customer_id", customerID),
		)
		s.appendHistory(session, query, resp.Answer, nil, nil)
		return s.buildResponse(resp, nil), nil
	}

	// Guarda: bloquear onboarding para usuários autenticados.
	// Abertura de conta só pode acontecer no endpoint anônimo (POST /v1/chat).
	// Só bloqueia quando o agente de fato inicia um fluxo (step == welcome ou campo de onboarding).
	if isAuthenticated && (step == "welcome" || resp.Context == "onboarding") {
		s.logger.Warn("⛔ onboarding bloqueado — usuário já autenticado",
			zap.String("customer_id", customerID),
			zap.String("step", step),
			zap.String("context", resp.Context),
			zap.Bool("is_authenticated", isAuthenticated),
		)
		return &FrontendResponse{
			Answer:  "Você já possui uma conta ativa. Posso ajudar com outra coisa?",
			Context: "geral",
		}, nil
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

	// Caso 2b: reset → agente pediu para resetar a sessão
	// Frontend deve limpar o state local e gerar novo customerID.
	if step == "reset" || nextStep == "reset" {
		s.logger.Info("🔄 reset solicitado pelo agente — limpando sessão",
			zap.String("customer_id", customerID),
		)
		if err := s.repo.DeleteSession(ctx, customerID); err != nil {
			s.logger.Error("failed to delete session from db", zap.Error(err))
		}
		s.sessions.Delete(customerID)
		resetStep := strPtr("reset")
		return &FrontendResponse{
			Answer:   sanitizeAnswer(resp.Answer),
			Context:  resp.Context,
			Step:     resetStep,
			NextStep: resetStep,
		}, nil
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
				// Enviamos a query original + validation_error especial para que o agente:
				// 1. Não rejeite por input vazio
				// 2. Saiba que o BFA já aceitou e salvou o campo
				// 3. Avance para o próximo campo sem re-validar
				retryResp, retryErr := s.callAgent(ctx, customerID, query, session,
					"CAMPO_ACEITO_BFA: o campo '"+step+"' foi validado e salvo pelo BFA. Avance para o próximo campo.", nil, isAuthenticated)
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

			// Se excedeu MaxRetries, resetar sessão e sinalizar ao frontend
			if session.Retries >= MaxRetries {
				s.logger.Warn("🔄 max retries exceeded — resetting session",
					zap.String("customer_id", customerID),
					zap.String("step", step),
					zap.Int("retries", session.Retries),
				)
				if err := s.repo.DeleteSession(ctx, customerID); err != nil {
					s.logger.Error("failed to delete session from db on reset", zap.Error(err))
				}
				s.sessions.Delete(customerID)
				resetStep := strPtr("reset")
				return &FrontendResponse{
					Answer:   fmt.Sprintf("Não conseguimos validar o campo após %d tentativas. Por favor, recomece o processo.", MaxRetries),
					Context:  resp.Context,
					Step:     resetStep,
					NextStep: resetStep,
				}, nil
			}

			errorResp, retryErr := s.callAgent(ctx, customerID, query, session, validationErr.Error(), nil, isAuthenticated)
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

		return s.checkCompletion(ctx, customerID, query, session, resp, isAuthenticated)
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

		// Se excedeu MaxRetries, resetar sessão e sinalizar ao frontend
		if session.Retries >= MaxRetries {
			s.logger.Warn("🔄 max retries exceeded — resetting session",
				zap.String("customer_id", customerID),
				zap.String("step", step),
				zap.Int("retries", session.Retries),
			)
			if err := s.repo.DeleteSession(ctx, customerID); err != nil {
				s.logger.Error("failed to delete session from db on reset", zap.Error(err))
			}
			s.sessions.Delete(customerID)
			resetStep := strPtr("reset")
			return &FrontendResponse{
				Answer:   fmt.Sprintf("Não conseguimos validar o campo após %d tentativas. Por favor, recomece o processo.", MaxRetries),
				Context:  resp.Context,
				Step:     resetStep,
				NextStep: resetStep,
			}, nil
		}

		// Reenviar ao agente com validation_error para que reformate
		errorResp, retryErr := s.callAgent(ctx, customerID, query, session, err.Error(), nil, isAuthenticated)
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
	return s.checkCompletion(ctx, customerID, query, session, resp, isAuthenticated)
}

// checkCompletion verifica se o agente retornou next_step=completed.
// Se sim e todos os campos estão preenchidos, finaliza a conta.
// Caso contrário, faz pass-through da resposta do agente.
func (s *Service) checkCompletion(ctx context.Context, customerID, query string, session *Session, resp *AgentResponse, isAuthenticated bool) (*FrontendResponse, error) {
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

		retryResp, retryErr := s.callAgent(ctx, customerID, query, session, validationMsg, nil, isAuthenticated)
		if retryErr != nil {
			return nil, fmt.Errorf("agent retry missing fields: %w", retryErr)
		}
		return s.buildResponse(retryResp, nil), nil
	}

	// Todos os campos preenchidos — disparar avaliação LLM-as-Judge ANTES de finalizar
	// (depois do FinalizeAccount o customerID temporário é perdido)
	s.evaluateAsync(customerID)

	// Finalizar conta
	accountData, err := s.repo.FinalizeAccount(ctx, customerID, session.OnboardingData)
	if err != nil {
		s.logger.Error("finalize account failed", zap.Error(err))
		return nil, fmt.Errorf("finalize account: %w", err)
	}
	// Log detalhado dos dados do cliente usados para abertura de conta
	logFields := []zap.Field{
		zap.String("customer_id", accountData.CustomerID),
		zap.String("agencia", accountData.Agencia),
		zap.String("conta", accountData.Conta),
	}
	for _, field := range RequiredOnboardingFields {
		val, ok := session.OnboardingData[field]
		if !ok {
			continue
		}
		if field == "password" || field == "passwordConfirmation" {
			logFields = append(logFields, zap.String(field, "******"))
		} else {
			logFields = append(logFields, zap.String(field, val))
		}
	}
	s.logger.Info("🎉 onboarding completed — dados do cliente", logFields...)

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
// Sanitiza o answer para remover textos técnicos internos (CAMPO_ACEITO_BFA, etc).
func (s *Service) buildResponse(resp *AgentResponse, accountData *AccountData) *FrontendResponse {
	return &FrontendResponse{
		Answer:      sanitizeAnswer(resp.Answer),
		Context:     resp.Context,
		Step:        resp.Step,
		NextStep:    resp.NextStep,
		AccountData: accountData,
	}
}

// sanitizeAnswer remove textos técnicos internos do answer antes de enviar ao frontend.
// Isso protege contra o agente Python colar o validation_error no answer.
func sanitizeAnswer(answer string) string {
	// Remover linhas que contenham sinais internos do BFA
	lines := strings.Split(answer, "\n")
	var clean []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "CAMPO_ACEITO_BFA") {
			continue
		}
		clean = append(clean, line)
	}
	result := strings.TrimSpace(strings.Join(clean, "\n"))
	if result == "" {
		return "Dado recebido! Continuando..."
	}
	return result
}

// saveTranscriptAsync salva a transcrição do turno de forma assíncrona.
// Fire-and-forget — erros são apenas logados, nunca bloqueiam o chat.
func (s *Service) saveTranscriptAsync(customerID, query string, resp *AgentResponse, latencyMs int64, bfaLatencyMs int64, errOccurred bool, financialCtx *FinancialContext) {
	ragCtx := resp.RagContexts
	if ragCtx == nil {
		ragCtx = []string{}
	}

	// Estimar tokens: ~4 chars por token (heurística razoável para português)
	tokensUsed := (len(query) + len(resp.Answer)) / 4

	t := Transcript{
		CustomerID:    customerID,
		Query:         query,
		Answer:        resp.Answer,
		RagContexts:   ragCtx,
		Step:          derefStr(resp.Step),
		Intent:        derefStr(resp.Intent),
		Confidence:    resp.Confidence,
		LatencyMs:     latencyMs,
		BfaLatencyMs:  bfaLatencyMs,
		ErrorOccurred: errOccurred,
		TokensUsed:    tokensUsed,
	}

	// Serializar contexto financeiro para persistência
	if financialCtx != nil {
		t.FinancialContextKeys = financialCtx.ContextKeys
		if raw, err := json.Marshal(financialCtx); err == nil {
			t.FinancialContextRaw = string(raw)
		}
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.transcripts.SaveTranscript(ctx, t); err != nil {
			s.logger.Warn("failed to save transcript (async)",
				zap.String("customer_id", customerID),
				zap.Error(err),
			)
		}
	}()
}

// evaluateAsync dispara a avaliação LLM-as-Judge de forma assíncrona.
// Chamado ANTES do FinalizeAccount para garantir que o customerID temporário
// ainda existe e todos os turnos estão no llm_transcripts.
// Fire-and-forget — erros são apenas logados, nunca bloqueiam a abertura de conta.
func (s *Service) evaluateAsync(customerID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// 1. Buscar todos os turnos da conversa
		transcripts, err := s.transcripts.ListTranscripts(ctx, customerID)
		if err != nil {
			s.logger.Warn("evaluate: failed to list transcripts",
				zap.String("customer_id", customerID),
				zap.Error(err),
			)
			return
		}

		if len(transcripts) == 0 {
			s.logger.Warn("evaluate: no transcripts found, skipping",
				zap.String("customer_id", customerID),
			)
			return
		}

		// 2. Montar o EvaluateRequest
		conversation := make([]TranscriptEntry, len(transcripts))
		for i, t := range transcripts {
			conversation[i] = TranscriptEntry{
				Query:                t.Query,
				Answer:               t.Answer,
				Contexts:             t.RagContexts,
				Step:                 t.Step,
				Intent:               t.Intent,
				Confidence:           t.Confidence,
				LatencyMs:            t.LatencyMs,
				CreatedAt:            t.CreatedAt,
				FinancialContextKeys: t.FinancialContextKeys,
			}
		}

		evalReq := EvaluateRequest{
			CustomerID:   customerID,
			Conversation: conversation,
		}

		s.logger.Info("📊 disparando LLM-as-Judge antes de finalizar conta",
			zap.String("customer_id", customerID),
			zap.Int("turns", len(conversation)),
		)

		// 3. Chamar o agente para avaliação
		evalResp, err := s.client.Evaluate(ctx, evalReq)
		if err != nil {
			s.logger.Warn("evaluate: agent call failed",
				zap.String("customer_id", customerID),
				zap.Error(err),
			)
			return
		}

		// 4. Salvar resultado no banco
		if err := s.evaluations.SaveEvaluation(ctx, *evalResp); err != nil {
			s.logger.Warn("evaluate: failed to save evaluation",
				zap.String("customer_id", customerID),
				zap.Error(err),
			)
			return
		}

		// 5. Marcar transcrições como avaliadas (não reenvia na próxima vez)
		if err := s.transcripts.MarkTranscriptsEvaluated(ctx, customerID); err != nil {
			s.logger.Warn("evaluate: failed to mark transcripts as evaluated",
				zap.String("customer_id", customerID),
				zap.Error(err),
			)
		}

		s.logger.Info("✅ LLM-as-Judge avaliação salva com sucesso",
			zap.String("customer_id", customerID),
			zap.Float64("overall_score", evalResp.OverallScore),
			zap.String("verdict", evalResp.Verdict),
			zap.Int("criteria", len(evalResp.Criteria)),
		)
	}()
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
