// Package service — chat_strategy_onboarding.go implementa a strategy
// de abertura de conta PJ (onboarding) via chat.
//
// ============================================================
// CONTRATO v9.0.0 — Onboarding com IA conversacional
// ============================================================
//
// O agente Python é a camada conversacional — interpreta linguagem natural,
// guia o cliente campo a campo e gera mensagens amigáveis.
//
// O BFA (Go) é a camada de negócio — valida formatos, aplica regras,
// persiste dados e controla o fluxo.
//
// Fluxo por turno:
//  1. BFA recebe query do frontend
//  2. BFA envia query + history enriquecido (step/validated) ao agente
//  3. Agente responde com step + field_value + next_step + answer
//  4. BFA valida o campo (se aplicável)
//  5. BFA enriquece o history com step + validated (true/false)
//  6. Se validação falhar → reenvia ao agente com validation_error
//  7. Se validação passar → persiste campo, devolve answer
//  8. Se next_step == "completed" → cria a conta
//
// O agente controla retries automaticamente (MAX_RETRIES = 3).
// O BFA NÃO precisa contar retries — o agente faz isso via history.
//
// Sequência de steps:
//
//	welcome → cnpj → razaoSocial → nomeFantasia → email
//	→ representanteName → representanteCpf → representantePhone
//	→ representanteBirthDate → password → passwordConfirmation → completed
package service

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/port"
	maindomain "github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	mainport "github.com/boddenberg/pj-assistant-bfa-go/internal/port"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost é o custo do bcrypt para hashing de senhas.
const bcryptCost = 10

// ============================================================
// OnboardingStrategy
// ============================================================

// OnboardingStrategy implementa ChatStrategy para o fluxo de abertura
// de conta PJ. Chama o agente em toda interação e valida cada campo.
type OnboardingStrategy struct {
	agentClient port.ChatAgentCaller
	authStore   mainport.AuthStore
	logger      *zap.Logger

	mu       sync.RWMutex
	sessions map[string]*domain.OnboardingSession
}

// NewOnboardingStrategy cria uma nova strategy de onboarding.
func NewOnboardingStrategy(
	agentClient port.ChatAgentCaller,
	authStore mainport.AuthStore,
	logger *zap.Logger,
) *OnboardingStrategy {
	return &OnboardingStrategy{
		agentClient: agentClient,
		authStore:   authStore,
		logger:      logger,
		sessions:    make(map[string]*domain.OnboardingSession),
	}
}

// CanHandle retorna true quando o intent é "onboarding" OU quando
// já existe uma sessão de onboarding ativa para esse customerID.
func (s *OnboardingStrategy) CanHandle(intent string, customerID string) bool {
	if intent == "onboarding" {
		return true
	}
	s.mu.RLock()
	session, ok := s.sessions[customerID]
	s.mu.RUnlock()
	return ok && session.Started
}

// ============================================================
// Handle — ponto de entrada
// ============================================================

func (s *OnboardingStrategy) Handle(ctx context.Context, chatCtx *domain.ChatContext) (*domain.ChatResponse, error) {
	ctx, span := chatTracer.Start(ctx, "OnboardingStrategy.Handle")
	defer span.End()

	session := s.getOrCreateSession(chatCtx.CustomerID)

	s.logger.Info("onboarding: processing message",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("query", truncate(chatCtx.Query, 50)),
		zap.Bool("started", session.Started),
		zap.Int("collected", len(session.CollectedData)),
	)

	// Salva a query para o history
	session.LastQuery = chatCtx.Query

	// Monta o request para o agente usando o history enriquecido da sessão
	agentReq := &domain.ChatAgentRequest{
		Query:      chatCtx.Query,
		CustomerID: chatCtx.CustomerID,
		Context:    "onboarding",
		History:    session.EnrichedHistory,
	}

	s.logger.Info("onboarding: calling agent",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.Int("history_len", len(session.EnrichedHistory)),
		zap.Strings("collected", session.CollectedFieldNames()),
	)

	// Chama o agente
	agentResp, err := s.agentClient.SendChat(ctx, agentReq)
	if err != nil {
		s.logger.Error("onboarding: agent call failed",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("onboarding agent call: %w", err)
	}

	s.logger.Info("onboarding: agent responded",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("step", stringPtrOrNil(agentResp.Step)),
		zap.String("field_value", stringPtrOrNil(agentResp.FieldValue)),
		zap.String("next_step", stringPtrOrNil(agentResp.NextStep)),
		zap.Int("answer_len", len(agentResp.Answer)),
	)

	// Processa a resposta
	return s.processAgentResponse(ctx, agentResp, session, chatCtx)
}

// ============================================================
// processAgentResponse — decide o que fazer com step
// ============================================================

func (s *OnboardingStrategy) processAgentResponse(
	ctx context.Context,
	resp *domain.ChatAgentResponse,
	session *domain.OnboardingSession,
	chatCtx *domain.ChatContext,
) (*domain.ChatResponse, error) {

	// Se step é nil → não é onboarding, tratar normalmente
	if resp.Step == nil {
		return s.buildResponse(resp), nil
	}

	step := *resp.Step

	switch step {
	case "welcome":
		// Agente deu boas-vindas → marcar sessão como iniciada
		session.Started = true

		// Adicionar turno ao history SEM step (welcome)
		session.EnrichedHistory = append(session.EnrichedHistory, domain.HistoryEntry{
			Query:     session.LastQuery,
			Answer:    resp.Answer,
			Step:      nil,
			Validated: nil,
		})

		s.logger.Info("onboarding: welcome received",
			zap.String("customer_id", chatCtx.CustomerID),
		)
		return s.buildResponse(resp), nil

	case "completed":
		// Agente sinalizou que terminou → finalizar cadastro
		s.logger.Info("onboarding: completed signal received, finalizing",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.Int("fields_count", len(session.CollectedData)),
		)
		return s.finalizeAccount(ctx, resp, session, chatCtx.CustomerID)

	default:
		// Campo de dados → validar
		return s.handleFieldValidation(ctx, resp, session, chatCtx)
	}
}

// ============================================================
// handleFieldValidation — valida o campo e persiste ou reenvia
// ============================================================

func (s *OnboardingStrategy) handleFieldValidation(
	ctx context.Context,
	resp *domain.ChatAgentResponse,
	session *domain.OnboardingSession,
	chatCtx *domain.ChatContext,
) (*domain.ChatResponse, error) {

	step := *resp.Step

	// Se field_value é nil → agente está pedindo o campo pela primeira vez
	if resp.FieldValue == nil {
		s.logger.Info("onboarding: agent asking for field (no value yet)",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.String("step", step),
		)
		return s.buildResponse(resp), nil
	}

	value := *resp.FieldValue

	// Fallback: se o agent mascarou o valor (PII protection)
	if strings.Contains(value, "***") || strings.Contains(value, "REDACTED") || strings.Contains(value, "[") {
		s.logger.Info("onboarding: field_value masked, using raw query",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.String("step", step),
			zap.String("masked_value", truncate(value, 30)),
		)
		value = strings.TrimSpace(chatCtx.Query)
	}

	s.logger.Info("onboarding: validating field",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("step", step),
		zap.String("value", truncate(value, 20)),
	)

	// Valida o campo
	validationErr := s.validateField(step, value, session)

	if validationErr != nil {
		// ── REJEITADO ──
		s.logger.Info("onboarding: field rejected",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.String("step", step),
			zap.String("error", validationErr.Error()),
		)

		// Adicionar turno ao history com validated=false
		session.EnrichedHistory = append(session.EnrichedHistory, domain.HistoryEntry{
			Query:     session.LastQuery,
			Answer:    resp.Answer,
			Step:      &step,
			Validated: domain.BoolPtr(false),
		})

		// Reenviar ao agente com validation_error
		retryReq := &domain.ChatAgentRequest{
			Query:           session.LastQuery,
			CustomerID:      chatCtx.CustomerID,
			Context:         "onboarding",
			History:         session.EnrichedHistory,
			ValidationError: validationErr.Error(),
		}

		s.logger.Info("onboarding: retrying agent with validation_error",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.String("step", step),
			zap.String("validation_error", validationErr.Error()),
		)

		retryResp, err := s.agentClient.SendChat(ctx, retryReq)
		if err != nil {
			s.logger.Error("onboarding: retry agent call failed",
				zap.String("customer_id", chatCtx.CustomerID),
				zap.Error(err),
			)
			return nil, fmt.Errorf("onboarding retry: %w", err)
		}

		s.logger.Info("onboarding: retry agent responded",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.String("step", stringPtrOrNil(retryResp.Step)),
			zap.Int("answer_len", len(retryResp.Answer)),
		)

		return s.buildResponse(retryResp), nil
	}

	// ── ACEITO ──

	// Formatar CNPJ/CPF antes de persistir
	if step == "cnpj" {
		value = formatCNPJ(onlyDigits(value))
	} else if step == "representanteCpf" {
		value = formatCPF(onlyDigits(value))
	}
	session.CollectedData[step] = value

	// Adicionar turno ao history com validated=true
	session.EnrichedHistory = append(session.EnrichedHistory, domain.HistoryEntry{
		Query:     session.LastQuery,
		Answer:    resp.Answer,
		Step:      &step,
		Validated: domain.BoolPtr(true),
	})

	s.logger.Info("onboarding: field accepted",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("step", step),
		zap.Int("total_fields", len(session.CollectedData)),
	)

	// Se next_step == "completed" ou último campo aceito → finalizar
	if (resp.NextStep != nil && *resp.NextStep == "completed") || step == "passwordConfirmation" {
		s.logger.Info("onboarding: last field accepted, auto-finalizing",
			zap.String("customer_id", chatCtx.CustomerID),
		)
		return s.finalizeAccount(ctx, resp, session, chatCtx.CustomerID)
	}

	return s.buildResponse(resp), nil
}

// ============================================================
// validateField — validação por campo
// ============================================================

func (s *OnboardingStrategy) validateField(field, value string, session *domain.OnboardingSession) error {
	switch field {
	case "cnpj":
		digits := onlyDigits(value)
		if len(digits) != 14 {
			return fmt.Errorf("CNPJ inválido: deve conter 14 dígitos numéricos (com ou sem pontuação)")
		}

	case "razaoSocial":
		if len(strings.TrimSpace(value)) < 3 {
			return fmt.Errorf("Razão Social deve ter no mínimo 3 caracteres")
		}

	case "nomeFantasia":
		if len(strings.TrimSpace(value)) < 2 {
			return fmt.Errorf("Nome Fantasia deve ter no mínimo 2 caracteres")
		}

	case "email":
		if _, err := mail.ParseAddress(value); err != nil {
			return fmt.Errorf("E-mail inválido: deve conter @ e um domínio válido")
		}

	case "representanteName":
		if len(strings.TrimSpace(value)) < 5 {
			return fmt.Errorf("Nome do representante deve ter no mínimo 5 caracteres")
		}

	case "representanteCpf":
		digits := onlyDigits(value)
		if len(digits) != 11 {
			return fmt.Errorf("CPF inválido: deve conter 11 dígitos numéricos (com ou sem pontuação)")
		}

	case "representantePhone":
		digits := onlyDigits(value)
		if len(digits) < 10 {
			return fmt.Errorf("Telefone inválido: deve conter no mínimo 10 dígitos no formato (XX) XXXXX-XXXX")
		}

	case "representanteBirthDate":
		date, err := time.Parse("02/01/2006", value)
		if err != nil {
			return fmt.Errorf("Data inválida: use o formato DD/MM/AAAA")
		}
		if calculateAge(date) < 18 {
			return fmt.Errorf("Representante deve ter no mínimo 18 anos")
		}

	case "password":
		if !regexp.MustCompile(`^\d{6}$`).MatchString(value) {
			return fmt.Errorf("Senha deve ter exatamente 6 dígitos numéricos, sem letras ou caracteres especiais")
		}

	case "passwordConfirmation":
		savedPassword, ok := session.CollectedData["password"]
		if !ok {
			return fmt.Errorf("Erro interno: senha anterior não encontrada. Reinicie o cadastro")
		}
		if value != savedPassword {
			return fmt.Errorf("As senhas não coincidem. Digite a mesma senha de 6 dígitos")
		}
	}

	return nil
}

// ============================================================
// finalizeAccount — cria a conta usando o AuthStore
// ============================================================

func (s *OnboardingStrategy) finalizeAccount(
	ctx context.Context,
	agentResp *domain.ChatAgentResponse,
	session *domain.OnboardingSession,
	customerID string,
) (*domain.ChatResponse, error) {

	completed := "completed"

	if s.authStore == nil {
		s.logger.Error("onboarding: authStore not available for registration")
		resp := s.buildResponse(agentResp)
		resp.Answer = "Todos os dados foram coletados com sucesso! ✅\n\n" +
			"Porém o serviço de cadastro está temporariamente indisponível. " +
			"Tente novamente em alguns instantes."
		resp.Step = &completed
		return resp, nil
	}

	data := session.CollectedData

	registerReq := &maindomain.RegisterRequest{
		CNPJ:                   onlyDigits(data["cnpj"]),
		RazaoSocial:            data["razaoSocial"],
		NomeFantasia:           data["nomeFantasia"],
		Email:                  data["email"],
		RepresentanteName:      data["representanteName"],
		RepresentanteCPF:       onlyDigits(data["representanteCpf"]),
		RepresentantePhone:     data["representantePhone"],
		RepresentanteBirthDate: data["representanteBirthDate"],
		Password:               data["password"],
	}

	// Verifica se CNPJ já existe
	existing, err := s.authStore.GetCustomerByDocument(ctx, registerReq.CNPJ)
	if err != nil {
		s.logger.Error("onboarding: failed to check existing customer", zap.Error(err))
		return nil, fmt.Errorf("check existing customer: %w", err)
	}
	if existing != nil {
		errorStep := "error"
		s.deleteSession(customerID)
		return &domain.ChatResponse{
			Answer: "⚠️ Esse CNPJ já está cadastrado no sistema. " +
				"Se você já tem conta, faça login. Se acredita que houve um erro, " +
				"entre em contato com nosso atendimento.",
			Context:    "onboarding",
			Intent:     "open_account",
			Confidence: 1.0,
			Step:       &errorStep,
		}, nil
	}

	// Hash da senha
	hash, err := bcrypt.GenerateFromPassword([]byte(registerReq.Password), bcryptCost)
	if err != nil {
		s.logger.Error("onboarding: failed to hash password", zap.Error(err))
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Cria cliente + conta + credenciais
	registerResp, err := s.authStore.CreateCustomerWithAccount(ctx, registerReq, string(hash))
	if err != nil {
		s.logger.Error("onboarding: failed to create customer", zap.Error(err))
		return nil, fmt.Errorf("create customer: %w", err)
	}

	s.logger.Info("onboarding: account created successfully",
		zap.String("customer_id", registerResp.CustomerID),
		zap.String("cnpj", registerReq.CNPJ),
		zap.String("agencia", registerResp.Agencia),
		zap.String("conta", registerResp.Conta),
	)

	s.deleteSession(customerID)

	return &domain.ChatResponse{
		Answer: fmt.Sprintf(
			"🎉 Conta criada com sucesso!\n\n"+
				"📋 Dados da sua conta:\n"+
				"• Agência: %s\n"+
				"• Conta: %s\n\n"+
				"Você já pode fazer login usando o CPF do representante e a senha de 6 dígitos que cadastrou.\n\n"+
				"Seja bem-vindo ao Itaú PJ! 🏦",
			registerResp.Agencia,
			registerResp.Conta,
		),
		Context:    "onboarding",
		Intent:     "open_account",
		Confidence: 1.0,
		Step:       &completed,
		AccountData: &domain.AccountData{
			CustomerID: registerResp.CustomerID,
			Agencia:    registerResp.Agencia,
			Conta:      registerResp.Conta,
		},
	}, nil
}

// ============================================================
// Helpers
// ============================================================

func (s *OnboardingStrategy) buildResponse(resp *domain.ChatAgentResponse) *domain.ChatResponse {
	return &domain.ChatResponse{
		Answer:           resp.Answer,
		Context:          resp.Context,
		Intent:           resp.Intent,
		Confidence:       resp.Confidence,
		Step:             resp.Step,
		FieldValue:       resp.FieldValue,
		NextStep:         resp.NextStep,
		SuggestedActions: resp.SuggestedActions,
	}
}

func (s *OnboardingStrategy) getOrCreateSession(customerID string) *domain.OnboardingSession {
	s.mu.RLock()
	session, ok := s.sessions[customerID]
	s.mu.RUnlock()

	if ok {
		return session
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok = s.sessions[customerID]; ok {
		return session
	}

	session = &domain.OnboardingSession{
		CollectedData:   make(map[string]string),
		EnrichedHistory: make([]domain.HistoryEntry, 0),
	}
	s.sessions[customerID] = session
	return session
}

func (s *OnboardingStrategy) deleteSession(customerID string) {
	s.mu.Lock()
	delete(s.sessions, customerID)
	s.mu.Unlock()
}

// formatCNPJ formata 14 dígitos como XX.XXX.XXX/XXXX-XX.
func formatCNPJ(digits string) string {
	if len(digits) != 14 {
		return digits
	}
	return fmt.Sprintf("%s.%s.%s/%s-%s", digits[:2], digits[2:5], digits[5:8], digits[8:12], digits[12:])
}

// formatCPF formata 11 dígitos como XXX.XXX.XXX-XX.
func formatCPF(digits string) string {
	if len(digits) != 11 {
		return digits
	}
	return fmt.Sprintf("%s.%s.%s-%s", digits[:3], digits[3:6], digits[6:9], digits[9:])
}

// onlyDigits extrai apenas dígitos de uma string.
func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// calculateAge calcula a idade em anos a partir da data de nascimento.
func calculateAge(birthDate time.Time) int {
	now := time.Now()
	age := now.Year() - birthDate.Year()
	if now.YearDay() < birthDate.YearDay() {
		age--
	}
	return age
}

// truncate corta uma string em N caracteres para logging seguro.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// stringPtrOrNil retorna o valor do ponteiro ou "<nil>" se nil.
func stringPtrOrNil(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
