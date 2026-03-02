// Package service — chat_strategy_onboarding.go implementa a strategy
// de abertura de conta PJ (onboarding) com validação campo a campo.
//
// ============================================================
// CONTRATO v8.0.0 — BFA ↔ Agente Python
// ============================================================
//
// O agente Python conduz a conversa e devolve:
//   - current_field: qual campo do onboarding está sendo tratado
//   - field_value:   valor cru que o cliente digitou
//
// O BFA (aqui) valida o campo e decide:
//   - OK  → persiste na sessão, devolve answer ao cliente
//   - ERRO → reenvia ao agente com validation_error preenchido
//
// Sequência de campos (ordem fixa):
//
//	welcome → cnpj → razaoSocial → nomeFantasia → email
//	→ representanteName → representanteCpf → representantePhone
//	→ representanteBirthDate → password → passwordConfirmation → completed
//
// Quando current_field == "completed", o BFA chama CreateCustomerWithAccount()
// para criar a conta (mesmo fluxo do POST /v1/auth/register).
//
// IMPORTANTE: essa strategy NÃO substitui o POST /v1/auth/register.
// O POST /v1/auth/register continua existindo para quem quiser
// criar conta enviando todos os dados de uma vez.
// Essa strategy é para o fluxo CONVERSACIONAL via chat.
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
// OnboardingStrategy — Strategy para abertura de conta via chat
// ============================================================

// OnboardingStrategy implementa ChatStrategy para o fluxo de abertura
// de conta PJ. Ela intercepta mensagens com intent "onboarding",
// valida cada campo retornado pelo agente, e no final chama Register().
type OnboardingStrategy struct {
	agentClient port.ChatAgentCaller
	authStore   mainport.AuthStore // nil quando Supabase não configurado
	logger      *zap.Logger

	// sessions guarda o estado do onboarding por customerID.
	// Em produção deveria ser Redis/banco, mas para o MVP
	// usar memória é suficiente e mantém o código simples.
	mu       sync.RWMutex
	sessions map[string]*domain.OnboardingSession
}

// NewOnboardingStrategy cria uma nova strategy de onboarding.
// authStore pode ser nil — nesse caso o cadastro retorna erro amigável.
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

// CanHandle retorna true quando o intent é "onboarding".
func (s *OnboardingStrategy) CanHandle(intent string) bool {
	return intent == "onboarding"
}

// ============================================================
// Handle — ponto de entrada do fluxo de onboarding
// ============================================================

// Handle processa uma mensagem no contexto de abertura de conta.
//
// Fluxo:
//  1. Envia query ao Agent Python com context="onboarding"
//  2. Agent devolve current_field + field_value + answer
//  3. BFA valida o campo (validateField)
//  4. Se OK  → persiste na sessão, devolve answer ao cliente
//  5. Se ERRO → reenvia ao agent com validation_error preenchido
//  6. Se current_field == "completed" → cria a conta
func (s *OnboardingStrategy) Handle(ctx context.Context, chatCtx *domain.ChatContext) (*domain.ChatResponse, error) {
	ctx, span := chatTracer.Start(ctx, "OnboardingStrategy.Handle")
	defer span.End()

	s.logger.Info("onboarding: processing message",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.Int("query_len", len(chatCtx.Query)),
	)

	// Garante que existe uma sessão para esse cliente
	session := s.getOrCreateSession(chatCtx.CustomerID)

	// Monta o request para o Agent Python
	agentReq := &domain.ChatAgentRequest{
		Query:           chatCtx.Query,
		CustomerID:      chatCtx.CustomerID,
		Context:         "onboarding",
		History:         chatCtx.History,
		ValidationError: chatCtx.ValidationError,
	}

	// Chama o Agent Python
	agentResp, err := s.agentClient.SendChat(ctx, agentReq)
	if err != nil {
		s.logger.Error("onboarding: agent call failed",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("onboarding agent call: %w", err)
	}

	// Salva a query para possível reenvio (se validação falhar)
	session.LastQuery = chatCtx.Query

	// Processa a resposta do agente com base no current_field
	return s.processAgentResponse(ctx, agentResp, session, chatCtx)
}

// ============================================================
// processAgentResponse — decide o que fazer com current_field
// ============================================================

func (s *OnboardingStrategy) processAgentResponse(
	ctx context.Context,
	resp *domain.ChatAgentResponse,
	session *domain.OnboardingSession,
	chatCtx *domain.ChatContext,
) (*domain.ChatResponse, error) {

	// Se current_field é nil → não é onboarding, tratar normalmente
	if resp.CurrentField == nil {
		return s.buildResponse(resp), nil
	}

	field := *resp.CurrentField

	switch field {
	case "welcome":
		// Agente deu boas-vindas → marcar que onboarding iniciou
		session.Started = true
		s.logger.Info("onboarding: welcome received",
			zap.String("customer_id", chatCtx.CustomerID),
		)
		return s.buildResponse(resp), nil

	case "completed":
		// Todos os campos coletados → finalizar cadastro
		s.logger.Info("onboarding: all fields collected, finalizing",
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

	field := *resp.CurrentField

	// Se field_value é nil, o agente está pedindo o campo pela primeira vez
	if resp.FieldValue == nil {
		return s.buildResponse(resp), nil
	}

	value := *resp.FieldValue

	s.logger.Debug("onboarding: validating field",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("field", field),
		zap.String("value_preview", truncate(value, 20)),
	)

	// Valida o campo
	validationErr := s.validateField(field, value, session)

	if validationErr != nil {
		// REJEITADO → reenviar ao agente com validation_error
		s.logger.Info("onboarding: field rejected",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.String("field", field),
			zap.String("error", validationErr.Error()),
		)

		retryReq := &domain.ChatAgentRequest{
			Query:           session.LastQuery,
			CustomerID:      chatCtx.CustomerID,
			Context:         "onboarding",
			History:         chatCtx.History,
			ValidationError: validationErr.Error(),
		}

		retryResp, err := s.agentClient.SendChat(ctx, retryReq)
		if err != nil {
			return nil, fmt.Errorf("onboarding retry agent call: %w", err)
		}

		// Resposta do retry: o agente vai pedir o campo de novo com o erro humanizado
		return s.buildResponse(retryResp), nil
	}

	// ACEITO → persistir o campo na sessão
	session.CollectedData[field] = value

	s.logger.Info("onboarding: field accepted",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("field", field),
		zap.Int("total_fields", len(session.CollectedData)),
	)

	return s.buildResponse(resp), nil
}

// ============================================================
// validateField — validação por campo (regras do contrato v8)
// ============================================================

func (s *OnboardingStrategy) validateField(field, value string, session *domain.OnboardingSession) error {
	switch field {
	case "cnpj":
		digits := onlyDigits(value)
		if len(digits) != 14 {
			return fmt.Errorf("CNPJ inválido: deve conter 14 dígitos numéricos no formato XX.XXX.XXX/XXXX-XX")
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
			return fmt.Errorf("CPF inválido: deve conter 11 dígitos numéricos no formato XXX.XXX.XXX-XX")
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

	// Verifica se o authStore está disponível
	if s.authStore == nil {
		s.logger.Error("onboarding: authStore not available for registration")
		resp := s.buildResponse(agentResp)
		resp.Answer = "Todos os dados foram coletados com sucesso! ✅\n\n" +
			"Porém o serviço de cadastro está temporariamente indisponível. " +
			"Tente novamente em alguns instantes."
		return resp, nil
	}

	data := session.CollectedData

	// Monta o RegisterRequest com os dados coletados
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
		resp := s.buildResponse(agentResp)
		resp.Answer = "⚠️ Esse CNPJ já está cadastrado no sistema. " +
			"Se você já tem conta, faça login. Se acredita que houve um erro, " +
			"entre em contato com nosso atendimento."
		// Limpa a sessão pois o cadastro não pode prosseguir
		s.deleteSession(customerID)
		return resp, nil
	}

	// Hash da senha (mesmo custo do auth_registration.go)
	hash, err := bcrypt.GenerateFromPassword([]byte(registerReq.Password), bcryptCost)
	if err != nil {
		s.logger.Error("onboarding: failed to hash password", zap.Error(err))
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Cria cliente + conta + credenciais no Supabase
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

	// Limpa a sessão
	s.deleteSession(customerID)

	// Monta resposta de sucesso com dados da conta
	resp := s.buildResponse(agentResp)
	resp.Answer = fmt.Sprintf(
		"🎉 Conta criada com sucesso!\n\n"+
			"📋 Dados da sua conta:\n"+
			"• Agência: %s\n"+
			"• Conta: %s\n"+
			"• ID do cliente: %s\n\n"+
			"Você já pode fazer login usando o CPF do representante e a senha de 6 dígitos que cadastrou.\n\n"+
			"Seja bem-vindo ao Itaú PJ! 🏦",
		registerResp.Agencia,
		registerResp.Conta,
		registerResp.CustomerID,
	)

	return resp, nil
}

// ============================================================
// Helpers
// ============================================================

// getOrCreateSession retorna a sessão de onboarding do cliente.
// Thread-safe com double-check locking.
func (s *OnboardingStrategy) getOrCreateSession(customerID string) *domain.OnboardingSession {
	s.mu.RLock()
	session, ok := s.sessions[customerID]
	s.mu.RUnlock()

	if ok {
		return session
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check após lock exclusivo
	if session, ok = s.sessions[customerID]; ok {
		return session
	}

	session = &domain.OnboardingSession{
		CollectedData: make(map[string]string),
	}
	s.sessions[customerID] = session
	return session
}

// deleteSession remove a sessão de onboarding do cliente.
func (s *OnboardingStrategy) deleteSession(customerID string) {
	s.mu.Lock()
	delete(s.sessions, customerID)
	s.mu.Unlock()
}

// buildResponse monta a ChatResponse a partir da resposta do agente.
func (s *OnboardingStrategy) buildResponse(resp *domain.ChatAgentResponse) *domain.ChatResponse {
	return &domain.ChatResponse{
		Answer:           resp.Answer,
		Context:          resp.Context,
		Intent:           resp.Intent,
		Confidence:       resp.Confidence,
		CurrentField:     resp.CurrentField,
		FieldValue:       resp.FieldValue,
		SuggestedActions: resp.SuggestedActions,
	}
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
