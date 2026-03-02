// Package service — chat_strategy_onboarding.go implementa a strategy
// de abertura de conta PJ (onboarding) 100% determinística.
//
// ============================================================
// CONTRATO v9.0.0 — Onboarding sem IA
// ============================================================
//
// O BFA conduz toda a conversa de onboarding localmente:
//   - Mensagens pré-definidas para cada campo (boas-vindas, perguntas, confirmações)
//   - Validação campo a campo com regras fixas
//   - Criação da conta no final via AuthStore
//
// A IA é usada APENAS como fallback quando a validação rejeita o campo,
// para gerar uma explicação mais amigável do erro (opcional).
// Se o fallback falhar, o BFA usa a mensagem de erro padrão.
//
// Sequência de campos (ordem fixa):
//
//	welcome → cnpj → razaoSocial → nomeFantasia → email
//	→ representanteName → representanteCpf → representantePhone
//	→ representanteBirthDate → password → passwordConfirmation → completed
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
// Mensagens pré-definidas — zero IA
// ============================================================

// fieldPrompt contém a mensagem que pede cada campo ao usuário.
var fieldPrompt = map[string]string{
	"cnpj": "Para começar, me informe o **CNPJ** da sua empresa 🏢\n\n" +
		"Pode digitar com ou sem pontuação (ex: 12.345.678/0001-90 ou 12345678000190).",

	"razaoSocial": "Agora me diga a **Razão Social** da empresa 📋\n\n" +
		"É o nome oficial que consta no contrato social.",

	"nomeFantasia": "Qual o **Nome Fantasia** da empresa? 🏷️\n\n" +
		"É o nome comercial, como os clientes conhecem a empresa.",

	"email": "Informe o **e-mail de contato** da empresa 📧\n\n" +
		"Esse e-mail será usado para comunicações importantes sobre a conta.",

	"representanteName": "Agora precisamos dos dados do representante legal.\n\n" +
		"Qual o **nome completo** do representante? 👤",

	"representanteCpf": "Informe o **CPF** do representante 🪪\n\n" +
		"Pode digitar com ou sem pontuação (ex: 123.456.789-00 ou 12345678900).",

	"representantePhone": "Qual o **telefone** do representante? 📱\n\n" +
		"Formato: (XX) XXXXX-XXXX",

	"representanteBirthDate": "Qual a **data de nascimento** do representante? 🎂\n\n" +
		"Formato: DD/MM/AAAA",

	"password": "Quase lá! Crie uma **senha numérica de 6 dígitos** 🔐\n\n" +
		"Essa será a senha da sua conta. Use apenas números.",

	"passwordConfirmation": "Por favor, **confirme a senha** digitando os mesmos 6 dígitos 🔐",
}

// fieldConfirmation contém a mensagem de confirmação quando o campo é aceito.
var fieldConfirmation = map[string]string{
	"cnpj":                   "CNPJ recebido! ✅",
	"razaoSocial":            "Razão Social registrada! ✅",
	"nomeFantasia":           "Nome Fantasia anotado! ✅",
	"email":                  "E-mail confirmado! ✅",
	"representanteName":      "Nome do representante registrado! ✅",
	"representanteCpf":       "CPF do representante confirmado! ✅",
	"representantePhone":     "Telefone registrado! ✅",
	"representanteBirthDate": "Data de nascimento confirmada! ✅",
	"password":               "Senha cadastrada! ✅",
	"passwordConfirmation":   "Senha confirmada! ✅",
}

// fieldLabels mapeia campo → descrição amigável para logs e mensagens.
var fieldLabels = map[string]string{
	"cnpj":                   "CNPJ",
	"razaoSocial":            "Razão Social",
	"nomeFantasia":           "Nome Fantasia",
	"email":                  "e-mail de contato",
	"representanteName":      "nome completo do representante",
	"representanteCpf":       "CPF do representante",
	"representantePhone":     "telefone do representante",
	"representanteBirthDate": "data de nascimento do representante (DD/MM/AAAA)",
	"password":               "senha de 6 dígitos numéricos",
	"passwordConfirmation":   "confirmação da senha",
}

// welcomeMessage é a mensagem de boas-vindas do onboarding.
const welcomeMessage = "Olá! 👋 Que bom que você quer abrir uma **conta PJ** no Itaú!\n\n" +
	"Vou precisar de algumas informações para criar sua conta. " +
	"São 10 campos simples — vamos lá?\n\n"

// ============================================================
// OnboardingStrategy — 100% determinística, IA só no fallback
// ============================================================

// OnboardingStrategy implementa ChatStrategy para o fluxo de abertura
// de conta PJ. Todo o caminho feliz é determinístico (sem IA).
// A IA é chamada apenas como fallback para humanizar erros de validação.
type OnboardingStrategy struct {
	agentClient port.ChatAgentCaller // usado APENAS como fallback em erros
	authStore   mainport.AuthStore   // nil quando Supabase não configurado
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
// Handle — ponto de entrada (100% determinístico)
// ============================================================

// Handle processa uma mensagem no contexto de abertura de conta.
// Todo o caminho feliz é local — sem chamada ao Agent Python.
// A IA só é chamada quando a validação falha, como fallback.
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

	// ── Passo 1: Se onboarding não iniciou → boas-vindas + pedir CNPJ ──
	if !session.Started {
		session.Started = true
		welcome := "welcome"
		firstPrompt := fieldPrompt["cnpj"]

		s.logger.Info("onboarding: welcome",
			zap.String("customer_id", chatCtx.CustomerID),
		)

		return &domain.ChatResponse{
			Answer:       welcomeMessage + firstPrompt,
			Context:      "onboarding",
			Intent:       "open_account",
			Confidence:   1.0,
			CurrentField: &welcome,
		}, nil
	}

	// ── Passo 2: Qual campo estamos esperando? ──
	expectedField := session.NextExpectedField()

	s.logger.Info("onboarding: expecting field",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("expected_field", expectedField),
		zap.Strings("collected", session.CollectedFieldNames()),
	)

	// ── Passo 3: Já coletou tudo? → finalizar ──
	if expectedField == "completed" {
		return s.finalizeAccount(ctx, session, chatCtx.CustomerID)
	}

	// ── Passo 4: Pegar valor e validar ──
	value := strings.TrimSpace(chatCtx.Query)
	validationErr := s.validateField(expectedField, value, session)

	if validationErr != nil {
		// Campo rejeitado — usar IA como fallback para mensagem amigável
		s.logger.Info("onboarding: field rejected",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.String("field", expectedField),
			zap.String("value", truncate(value, 30)),
			zap.String("error", validationErr.Error()),
		)

		friendlyMsg := s.friendlyError(ctx, expectedField, value, validationErr, chatCtx)

		return &domain.ChatResponse{
			Answer:       friendlyMsg,
			Context:      "onboarding",
			Intent:       "open_account",
			Confidence:   1.0,
			CurrentField: &expectedField,
		}, nil
	}

	// ── Passo 5: Campo aceito → formatar e persistir ──
	if expectedField == "cnpj" {
		value = formatCNPJ(onlyDigits(value))
	} else if expectedField == "representanteCpf" {
		value = formatCPF(onlyDigits(value))
	}
	session.CollectedData[expectedField] = value

	s.logger.Info("onboarding: field accepted",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("field", expectedField),
		zap.String("value", truncate(value, 30)),
		zap.Int("total_fields", len(session.CollectedData)),
	)

	// ── Passo 6: Último campo? → criar conta direto ──
	if expectedField == "passwordConfirmation" {
		s.logger.Info("onboarding: all fields collected, auto-finalizing",
			zap.String("customer_id", chatCtx.CustomerID),
		)
		return s.finalizeAccount(ctx, session, chatCtx.CustomerID)
	}

	// ── Passo 7: Confirmação + pergunta do próximo campo ──
	nextField := session.NextExpectedField()
	confirmation := fieldConfirmation[expectedField]
	nextPrompt := fieldPrompt[nextField]

	answer := confirmation + "\n\n" + nextPrompt

	return &domain.ChatResponse{
		Answer:       answer,
		Context:      "onboarding",
		Intent:       "open_account",
		Confidence:   1.0,
		CurrentField: &expectedField,
		FieldValue:   &value,
	}, nil
}

// ============================================================
// friendlyError — fallback com IA para humanizar erros
// ============================================================

// friendlyError tenta chamar a IA para gerar uma mensagem amigável
// explicando o erro de validação. Se a IA falhar (timeout, indisponível),
// retorna uma mensagem padrão formatada.
func (s *OnboardingStrategy) friendlyError(
	ctx context.Context,
	field string,
	value string,
	validationErr error,
	chatCtx *domain.ChatContext,
) string {
	// Mensagem padrão (sem IA) — sempre funciona, zero latência
	defaultMsg := fmt.Sprintf(
		"⚠️ %s\n\nPor favor, tente novamente.\n\n%s",
		validationErr.Error(),
		fieldPrompt[field],
	)

	// Se não tem agentClient, retorna a mensagem padrão
	if s.agentClient == nil {
		return defaultMsg
	}

	// Tenta usar a IA como fallback para uma resposta mais humana
	agentReq := &domain.ChatAgentRequest{
		Query:           value,
		CustomerID:      chatCtx.CustomerID,
		Context:         "onboarding_error",
		History:         chatCtx.History,
		ValidationError: validationErr.Error(),
		ExpectedField:   field,
		CollectedFields: session2CollectedNames(chatCtx.CustomerID, s),
	}

	s.logger.Info("onboarding: calling AI fallback for friendly error",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("field", field),
		zap.String("error", validationErr.Error()),
	)

	agentResp, err := s.agentClient.SendChat(ctx, agentReq)
	if err != nil {
		s.logger.Warn("onboarding: AI fallback failed, using default message",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.Error(err),
		)
		return defaultMsg
	}

	if agentResp.Answer == "" {
		return defaultMsg
	}

	s.logger.Info("onboarding: AI fallback succeeded",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("field", field),
		zap.Int("answer_len", len(agentResp.Answer)),
	)

	return agentResp.Answer
}

// session2CollectedNames retorna os nomes dos campos já coletados.
func session2CollectedNames(customerID string, s *OnboardingStrategy) []string {
	s.mu.RLock()
	session, ok := s.sessions[customerID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return session.CollectedFieldNames()
}

// ============================================================
// validateField — validação por campo (regras do contrato v8)
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
	session *domain.OnboardingSession,
	customerID string,
) (*domain.ChatResponse, error) {

	completed := "completed"

	if s.authStore == nil {
		s.logger.Error("onboarding: authStore not available for registration")
		return &domain.ChatResponse{
			Answer: "Todos os dados foram coletados com sucesso! ✅\n\n" +
				"Porém o serviço de cadastro está temporariamente indisponível. " +
				"Tente novamente em alguns instantes.",
			Context:      "onboarding",
			Intent:       "open_account",
			Confidence:   1.0,
			CurrentField: &completed,
		}, nil
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
		errorField := "error"
		s.deleteSession(customerID)
		return &domain.ChatResponse{
			Answer: "⚠️ Esse CNPJ já está cadastrado no sistema. " +
				"Se você já tem conta, faça login. Se acredita que houve um erro, " +
				"entre em contato com nosso atendimento.",
			Context:      "onboarding",
			Intent:       "open_account",
			Confidence:   1.0,
			CurrentField: &errorField,
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
		Context:      "onboarding",
		Intent:       "open_account",
		Confidence:   1.0,
		CurrentField: &completed,
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
