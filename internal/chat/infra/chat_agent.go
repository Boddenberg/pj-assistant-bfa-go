package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/chat/domain"
	maindomain "github.com/boddenberg/pj-assistant-bfa-go/internal/domain"
	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/resilience"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// tracer é o tracer OpenTelemetry para o módulo chat/infra.
var tracer = otel.Tracer("chat/infra")

// ============================================================
// ChatAgentClient — cliente HTTP que chama o Agent Python
// ============================================================
//
// Diferente do AgentClient (que chama POST /v1/agent/invoke com payload pesado),
// este client chama POST /v1/chat com o contrato simples do agent Python:
//
//	Request:  {"query": "Como abrir conta PJ?"}
//	Response: {"answer": "...", "sources": [...], "tokens_used": 1250, ...}
//
// Esse client é usado exclusivamente pela rota GET /v1/assistant/{customerId}.

type ChatAgentClient struct {
	httpClient *http.Client
	baseURL    string // ex: https://pj-assistant-agent-py-production.up.railway.app
	cb         *gobreaker.CircuitBreaker
	cfg        resilience.Config
}

// NewChatAgentClient cria o client que se comunica com o Agent Python.
// O baseURL deve ser a URL base do agent (sem /v1/chat no final).
func NewChatAgentClient(httpClient *http.Client, baseURL string, cb *gobreaker.CircuitBreaker, cfg resilience.Config) *ChatAgentClient {
	return &ChatAgentClient{
		httpClient: httpClient,
		baseURL:    baseURL,
		cb:         cb,
		cfg:        cfg,
	}
}

// SendChat envia uma mensagem para o Agent Python e retorna a resposta.
//
// Fluxo:
//  1. Serializa o ChatAgentRequest como JSON
//  2. Faz POST para {baseURL}/v1/chat
//  3. Decodifica a resposta ChatAgentResponse
//  4. Usa circuit breaker + retry para resiliência
//
// O circuit breaker protege contra o agent estar fora do ar.
// O retry com backoff tenta novamente em caso de falha temporária.
func (c *ChatAgentClient) SendChat(ctx context.Context, req *domain.ChatAgentRequest) (*domain.ChatAgentResponse, error) {
	ctx, span := tracer.Start(ctx, "ChatAgentClient.SendChat")
	defer span.End()
	span.SetAttributes(attribute.String("customer.id", req.CustomerID))

	var agentResp domain.ChatAgentResponse

	// Executa com circuit breaker — se o agent estiver fora,
	// o breaker abre e retorna erro imediato nas próximas chamadas.
	result, err := c.cb.Execute(func() (any, error) {
		// Retry com backoff exponencial — tenta até cfg.MaxRetries vezes
		innerErr := resilience.RetryWithBackoff(ctx, c.cfg, func() error {
			// Serializa o request
			body, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("marshal chat request: %w", err)
			}

			// Monta o POST /v1/chat
			url := fmt.Sprintf("%s/v1/chat", c.baseURL)
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("create http request: %w", err)
			}
			httpReq.Header.Set("Content-Type", "application/json")

			// Faz a chamada HTTP
			resp, err := c.httpClient.Do(httpReq)
			if err != nil {
				return fmt.Errorf("http call to agent: %w", err)
			}
			defer resp.Body.Close()

			// Verifica status — qualquer coisa diferente de 200 é erro
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("agent /v1/chat returned status %d", resp.StatusCode)
			}

			// Decodifica a resposta do agent
			return json.NewDecoder(resp.Body).Decode(&agentResp)
		})

		if innerErr != nil {
			return nil, innerErr
		}
		return &agentResp, nil
	})

	if err != nil {
		return nil, &maindomain.ErrExternalService{Service: "chat-agent", Err: err}
	}

	return result.(*domain.ChatAgentResponse), nil
}
