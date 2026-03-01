// Package service — chat_strategy_onboarding.go implementa a strategy
// de abertura de conta PJ (onboarding).
//
// ============================================================
// JORNADA DE ABERTURA DE CONTA — 3 Etapas
// ============================================================
//
// A abertura de conta PJ é um fluxo conversacional de 3 etapas:
//
//	Etapa 1 — Dados da Empresa:
//	  → CNPJ, razão social, nome fantasia, email
//
//	Etapa 2 — Dados do Representante Legal:
//	  → Nome completo, CPF, telefone, data de nascimento
//
//	Etapa 3 — Senha:
//	  → Senha de 6 dígitos (numérica)
//
// O Agent Python conduz a conversa e extrai os dados. O BFA valida,
// armazena e no final chama o RegisterService para criar a conta.
//
// IMPORTANTE: essa strategy NÃO substitui o POST /v1/auth/register.
// O POST /v1/auth/register continua existindo para quem quiser
// criar conta de forma "manual" (enviando todos os dados de uma vez).
// Essa strategy é para o fluxo CONVERSACIONAL, onde o usuário
// vai respondendo perguntas uma a uma via chat.
package service

import (
	"context"
	"fmt"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/port"

	"go.uber.org/zap"
)

// ============================================================
// OnboardingStrategy — Strategy para abertura de conta via chat
// ============================================================

// OnboardingStrategy implementa a interface ChatStrategy para o contexto
// de abertura de conta PJ. Ela intercepta mensagens com intent "onboarding"
// e gerencia o fluxo conversacional de 3 etapas.
//
// Na versão atual (MVP), a strategy repassa a query pro Agent Python
// com o contexto "onboarding" para que o agent conduza a conversa.
// O agent retorna instruções em linguagem natural para o usuário.
//
// Em versões futuras, essa strategy pode:
//   - Extrair dados estruturados da resposta do agent
//   - Validar campos (CNPJ, CPF, etc.) no BFA antes de prosseguir
//   - Manter state machine com os dados coletados em cada etapa
//   - Chamar s.authService.Register() quando todas as etapas estiverem completas
type OnboardingStrategy struct {
	// agentClient chama o Agent Python para conduzir a conversa
	agentClient port.ChatAgentCaller

	// logger para logging estruturado
	logger *zap.Logger
}

// NewOnboardingStrategy cria uma nova strategy de onboarding.
func NewOnboardingStrategy(
	agentClient port.ChatAgentCaller,
	logger *zap.Logger,
) *OnboardingStrategy {
	return &OnboardingStrategy{
		agentClient: agentClient,
		logger:      logger,
	}
}

// CanHandle retorna true quando o intent é "onboarding".
// O ChatService chama esse método para decidir se essa strategy
// deve processar a mensagem.
func (s *OnboardingStrategy) CanHandle(intent string) bool {
	return intent == "onboarding"
}

// Handle processa uma mensagem no contexto de abertura de conta.
//
// Fluxo atual (MVP):
//  1. Monta o ChatAgentRequest com context="onboarding"
//  2. Se existir journey state, inclui no request para o agent
//  3. Envia pro Agent Python
//  4. Retorna a resposta como answer string
//
// O Agent Python é quem conduz a conversa, perguntando dados
// na ordem certa e orientando o usuário.
//
// Fluxo futuro (com state machine):
//  1. Verifica em qual etapa o usuário está (via JourneyState)
//  2. Extrai dados da mensagem usando o agent
//  3. Valida os dados no BFA
//  4. Se validação ok → avança para próxima etapa
//  5. Se etapa 3 completa → chama Register() e cria a conta
//  6. Retorna mensagem de confirmação
func (s *OnboardingStrategy) Handle(ctx context.Context, chatCtx *domain.ChatContext) (*domain.ChatResponse, error) {
	ctx, span := chatTracer.Start(ctx, "OnboardingStrategy.Handle")
	defer span.End()

	s.logger.Info("processing onboarding message",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("query", chatCtx.Query),
	)

	// Monta o request para o Agent Python com contexto de onboarding.
	// O campo "context" diz ao agent que estamos no fluxo de abertura de conta,
	// para que ele conduza a conversa de forma adequada.
	agentReq := &domain.ChatAgentRequest{
		Query:      chatCtx.Query,
		CustomerID: chatCtx.CustomerID,
		Context:    "onboarding",
	}

	// Se já existe uma jornada em andamento, inclui o state.
	// O agent usa isso para saber em qual etapa estamos e quais dados
	// já foram coletados.
	if chatCtx.Journey != nil {
		agentReq.JourneyState = chatCtx.Journey
	} else {
		// Primeira mensagem da jornada — cria um state inicial
		agentReq.JourneyState = &domain.JourneyState{
			JourneyType:   "onboarding",
			Stage:         1,
			Status:        "in_progress",
			CollectedData: make(map[string]string),
		}
	}

	// Chama o Agent Python
	agentResp, err := s.agentClient.SendChat(ctx, agentReq)
	if err != nil {
		s.logger.Error("agent call failed during onboarding",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("onboarding agent call: %w", err)
	}

	s.logger.Info("onboarding agent response received",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.Int("tokens_used", agentResp.TokensUsed),
	)

	// Retorna somente a answer string
	return &domain.ChatResponse{
		Answer: agentResp.Answer,
	}, nil
}
