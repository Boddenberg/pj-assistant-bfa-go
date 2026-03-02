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
	// Recebe também o customerID para que strategies com estado (ex: onboarding)
	// possam verificar se há sessão ativa para esse cliente.
	// Exemplos de intent: "onboarding", "pix", "balance", "general"
	CanHandle(intent string, customerID string) bool

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

	// maxHistory é o número máximo de entradas de histórico enviadas ao agent.
	// Controlado pela variável de ambiente CHAT_MAX_HISTORY (default: 5).
	maxHistory int

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
	maxHistory int,
	logger *zap.Logger,
) *ChatService {
	if maxHistory <= 0 {
		maxHistory = 5
	}
	return &ChatService{
		agentClient: agentClient,
		strategies:  strategies,
		maxHistory:  maxHistory,
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

	// Passo 2: Trunca o histórico para no máximo maxHistory entradas.
	// O frontend pode mandar N entradas, mas só repassamos as últimas
	// para economizar tokens e manter a janela de contexto sob controle.
	history := req.History
	if len(history) > s.maxHistory {
		history = history[len(history)-s.maxHistory:]
	}

	// Passo 3: Monta o contexto que a strategy vai receber
	chatCtx := &domain.ChatContext{
		CustomerID:     customerID,
		Query:          req.Query,
		DetectedIntent: intent,
		History:        history,
	}

	// Passo 4: Procura uma strategy registrada que aceite o intent
	// Passa customerID para que strategies com sessão ativa (ex: onboarding)
	// possam se identificar mesmo sem keyword match.
	for _, strategy := range s.strategies {
		if strategy.CanHandle(intent, customerID) {
			s.logger.Info("chat: delegating to strategy",
				zap.String("customer_id", customerID),
				zap.String("intent", intent),
				zap.Int("history_len", len(history)),
			)
			return strategy.Handle(ctx, chatCtx)
		}
	}

	// Passo 5: Nenhuma strategy encontrada → usa a default
	// A default simplesmente repassa a query direto pro agent
	s.logger.Info("chat: no strategy matched, using default agent call",
		zap.String("customer_id", customerID),
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
		History:    chatCtx.History,
	}

	// Chama o Agent Python
	s.logger.Info("chat: calling agent (default)",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("context", "general"),
	)
	agentResp, err := s.agentClient.SendChat(ctx, agentReq)
	if err != nil {
		s.logger.Error("chat: agent call failed",
			zap.String("customer_id", chatCtx.CustomerID),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Info("chat: agent responded (default)",
		zap.String("customer_id", chatCtx.CustomerID),
		zap.String("context", agentResp.Context),
		zap.String("intent", agentResp.Intent),
		zap.Float64("confidence", agentResp.Confidence),
		zap.Int("answer_len", len(agentResp.Answer)),
	)

	// Monta a resposta para o chamador
	return &domain.ChatResponse{
		Answer:           agentResp.Answer,
		Context:          agentResp.Context,
		Intent:           agentResp.Intent,
		Confidence:       agentResp.Confidence,
		Step:             agentResp.Step,
		FieldValue:       agentResp.FieldValue,
		NextStep:         agentResp.NextStep,
		SuggestedActions: agentResp.SuggestedActions,
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
