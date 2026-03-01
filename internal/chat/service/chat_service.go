// Package service — chat_service.go implementa o ChatService.
//
// ============================================================
// ARQUITETURA — Strategy Pattern para Routing de Contexto
// ============================================================
//
// O ChatService é o orquestrador central da rota GET /v1/assistant/{customerId}.
// Ele recebe a query do usuário, detecta a intenção (intent) e delega
// o processamento para a Strategy correta.
//
// Fluxo completo:
//  1. Handler recebe GET /v1/assistant/{customerId} com body {"query": "..."}
//  2. ChatService.ProcessMessage() é chamado
//  3. Detecta a intenção do usuário (onboarding? pix? geral?)
//  4. Busca a Strategy correspondente no mapa de strategies
//  5. Se não encontra, usa a DefaultStrategy (que manda pro agent direto)
//  6. A Strategy pode enriquecer o request, validar dados, gerenciar jornada
//  7. No final, chama o Agent Python (POST /v1/chat) e devolve a resposta
//
// Strategies disponíveis:
//   - DefaultStrategy: repassa a query direto pro agent (sem lógica extra)
//   - OnboardingStrategy: gerencia o fluxo de abertura de conta PJ em 3 etapas
//   - (futuro) PixStrategy, BillingStrategy, etc.
package service

import (
	"context"
	"strings"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/port"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// chatTracer é o tracer OpenTelemetry para o módulo de chat.
var chatTracer = otel.Tracer("chat/service")

// ============================================================
// ChatStrategy — interface que cada contexto implementa
// ============================================================

// ChatStrategy define o contrato de uma estratégia de processamento.
// Cada contexto (onboarding, pix, etc.) implementa sua própria strategy.
//
// CanHandle: diz se essa strategy sabe lidar com a intenção detectada
// Handle:    processa a mensagem e retorna a resposta da IA
type ChatStrategy interface {
	// CanHandle retorna true se essa strategy trata a intenção dada.
	// Exemplos de intent: "onboarding", "pix", "balance", "general"
	CanHandle(intent string) bool

	// Handle processa a mensagem do chat dentro do contexto dessa strategy.
	// Recebe o ChatContext com todas as informações necessárias.
	// Retorna a resposta do agent (answer string).
	Handle(ctx context.Context, chatCtx *domain.ChatContext) (*domain.ChatResponse, error)
}

// ============================================================
// ChatService — orquestrador com strategy routing
// ============================================================

// ChatService é o serviço principal da rota de chat.
// Ele usa o Strategy Pattern para rotear mensagens para o contexto correto.
type ChatService struct {
	// agentClient é o client HTTP que chama o Agent Python
	agentClient port.ChatAgentCaller

	// strategies é o mapa de strategies registradas.
	// A chave é o nome do contexto (ex: "onboarding").
	// Quando uma nova strategy é criada, basta registrá-la aqui.
	strategies []ChatStrategy

	// logger para logging estruturado
	logger *zap.Logger
}

// NewChatService cria o ChatService com as dependências injetadas.
//
// O parâmetro strategies recebe todas as strategies disponíveis.
// A ordem importa: a primeira strategy que aceita a intenção ganha.
func NewChatService(
	agentClient port.ChatAgentCaller,
	strategies []ChatStrategy,
	logger *zap.Logger,
) *ChatService {
	return &ChatService{
		agentClient: agentClient,
		strategies:  strategies,
		logger:      logger,
	}
}

// ProcessMessage é o ponto de entrada principal do chat.
//
// Fluxo:
//  1. Detecta a intenção do usuário baseado em palavras-chave
//  2. Monta o ChatContext com customerID, query e intent
//  3. Procura uma strategy que saiba lidar com o intent
//  4. Se nenhuma strategy aceita → usa a DefaultStrategy
//  5. Retorna a resposta da IA
func (s *ChatService) ProcessMessage(ctx context.Context, customerID string, req *domain.ChatRequest) (*domain.ChatResponse, error) {
	ctx, span := chatTracer.Start(ctx, "ChatService.ProcessMessage")
	defer span.End()

	// Passo 1: Detecta a intenção do usuário.
	// Por enquanto usa keywords simples. No futuro o Agent Python
	// pode fazer isso de forma mais sofisticada.
	intent := s.detectIntent(req.Query)

	s.logger.Info("chat message received",
		zap.String("customer_id", customerID),
		zap.String("intent", intent),
		zap.Int("query_length", len(req.Query)),
	)

	// Passo 2: Monta o contexto que a strategy vai receber
	chatCtx := &domain.ChatContext{
		CustomerID:     customerID,
		Query:          req.Query,
		DetectedIntent: intent,
		Journey:        nil, // futuro: buscar journey state do banco/cache
	}

	// Passo 3: Procura uma strategy registrada que aceite o intent
	for _, strategy := range s.strategies {
		if strategy.CanHandle(intent) {
			s.logger.Debug("delegating to strategy",
				zap.String("intent", intent),
			)
			return strategy.Handle(ctx, chatCtx)
		}
	}

	// Passo 4: Nenhuma strategy encontrada → usa a default
	// A default simplesmente repassa a query direto pro agent
	s.logger.Debug("no strategy matched, using default agent call",
		zap.String("intent", intent),
	)
	return s.defaultHandle(ctx, chatCtx)
}

// defaultHandle envia a query diretamente pro Agent Python sem lógica extra.
// É o fallback quando nenhuma strategy registrada aceita o intent.
func (s *ChatService) defaultHandle(ctx context.Context, chatCtx *domain.ChatContext) (*domain.ChatResponse, error) {
	// Monta o request para o Agent Python
	agentReq := &domain.ChatAgentRequest{
		Query:      chatCtx.Query,
		CustomerID: chatCtx.CustomerID,
		Context:    "general",
	}

	// Chama o Agent Python
	agentResp, err := s.agentClient.SendChat(ctx, agentReq)
	if err != nil {
		s.logger.Error("agent call failed",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.Error(err),
		)
		return nil, err
	}

	// Retorna somente a string answer
	return &domain.ChatResponse{
		Answer: agentResp.Answer,
	}, nil
}

// ============================================================
// detectIntent — detecção simples de intenção por keywords
// ============================================================

// detectIntent analisa a query do usuário e retorna uma string de intent.
//
// Keywords mapeadas:
//   - "abrir conta", "abertura", "onboarding", "cadastro" → "onboarding"
//   - "pix", "transferir", "transferência"                → "pix"
//   - "saldo", "extrato", "balance"                       → "balance"
//   - qualquer outra coisa                                → "general"
//
// No futuro isso pode ser substituído por um classificador ML no Agent.
func (s *ChatService) detectIntent(query string) string {
	lower := strings.ToLower(query)

	// Onboarding / abertura de conta
	onboardingKeywords := []string{
		"abrir conta", "abertura de conta", "abrir minha conta",
		"criar conta", "onboarding", "cadastro", "cadastrar",
		"quero abrir", "como abrir", "nova conta",
	}
	for _, kw := range onboardingKeywords {
		if strings.Contains(lower, kw) {
			return "onboarding"
		}
	}

	// Pix
	pixKeywords := []string{"pix", "transferir", "transferência", "transferencia", "enviar dinheiro"}
	for _, kw := range pixKeywords {
		if strings.Contains(lower, kw) {
			return "pix"
		}
	}

	// Saldo / extrato
	balanceKeywords := []string{"saldo", "extrato", "balance", "quanto tenho", "meu dinheiro"}
	for _, kw := range balanceKeywords {
		if strings.Contains(lower, kw) {
			return "balance"
		}
	}

	// Default: query genérica
	return "general"
}
