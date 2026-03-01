# 📘 Tutorial — Rota POST /v1/chat/{customerId} (Chat com IA)

## Visão Geral

A rota `POST /v1/chat/{customerId}` é a **porta de entrada do chat com IA** no BFA.
Ela permite que qualquer frontend/chatbot envie uma mensagem em linguagem natural
e receba uma resposta da IA, tudo de forma simples e leve.

> **Por que POST e não GET?** Proxies reversos (Railway, CloudFlare, etc.)
> removem o body de requisições GET, causando erro 400/500 em produção.
> POST é o método correto para enviar dados.

```
┌──────────┐    POST /v1/chat/{id}      ┌────────┐   POST /v1/chat    ┌────────────────┐
│ Frontend │ ─────────────────────────→ │  BFA   │ ─────────────────→ │ Agent Python   │
│ Chatbot  │ ←───────────────────────── │ (Go)   │ ←───────────────── │ (LangGraph)    │
└──────────┘    {"answer": "..."}          └────────┘   {"answer":"..."}  └────────────────┘
```

---

## Como Usar

### Request

```bash
curl -X POST \
  https://pj-assistant-bfa-go-production.up.railway.app/v1/chat/ab84533a-9589-41e1-b503-50cdc9cb9860 \
  -H "Content-Type: application/json" \
  -d '{"query": "Quero abrir uma conta PJ"}'
```

### Response (200 OK)

```json
{
  "answer": "Olá! Vou te ajudar a abrir sua conta PJ. Para começar, preciso de alguns dados da sua empresa: CNPJ, razão social, nome fantasia e email."
}
```

### Erros Possíveis

| Status | Motivo |
|--------|--------|
| 400 | `customer_id` ausente, body inválido, ou `query` vazia |
| 502 | Agent Python fora do ar ou retornou erro |
| 503 | Circuit breaker aberto (muitas falhas no agent) |

---

## Arquitetura

### Separação de Responsabilidades

| Componente | Responsabilidade |
|------------|-----------------|
| **BFA (Go)** | Validação, routing de contexto (Strategy Pattern), state management, persistência |
| **Agent Python** | Conversa com IA, NLU, RAG sobre knowledge base, geração de respostas |

O BFA **não faz IA**. Ele é o orquestrador que decide qual contexto tratar
e envia a query pro Agent Python com o contexto apropriado.

### Strategy Pattern

O coração da rota é o **Strategy Pattern** para routing de contexto:

```
                    ┌─────────────────┐
                    │  ChatService    │
                    │  (Orquestrador) │
                    └───────┬─────────┘
                            │
                    detectIntent(query)
                            │
              ┌─────────────┼─────────────┐
              │             │             │
              ▼             ▼             ▼
    ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
    │ Onboarding  │ │ Pix         │ │ Default     │
    │ Strategy    │ │ Strategy    │ │ (fallback)  │
    │ (abertura)  │ │ (futuro)    │ │ agent call  │
    └─────────────┘ └─────────────┘ └─────────────┘
```

**Como funciona:**

1. `ChatService.ProcessMessage()` recebe a query
2. `detectIntent()` analisa keywords e retorna um intent (ex: `"onboarding"`)
3. O service itera pelas strategies registradas
4. A primeira strategy que `CanHandle(intent) == true` processa a mensagem
5. Se nenhuma strategy aceita → fallback para chamada direta ao agent

### Detecção de Intent (keywords)

| Keywords | Intent |
|----------|--------|
| "abrir conta", "abertura", "cadastro", "onboarding" | `onboarding` |
| "pix", "transferir", "transferência" | `pix` |
| "saldo", "extrato", "balance" | `balance` |
| qualquer outra coisa | `general` |

---

## Arquivos Criados / Modificados

### Novos

| Arquivo | Descrição |
|---------|-----------|  
| `internal/chat/domain/chat.go` | Tipos de domínio: ChatRequest, ChatResponse, ChatAgentRequest, ChatAgentResponse, JourneyState, ChatContext |
| `internal/chat/port/chat_port.go` | Interface `ChatAgentCaller` — port para o agent client |
| `internal/chat/infra/chat_agent.go` | Client HTTP que chama `POST /v1/chat` no Agent Python (com circuit breaker + retry) |
| `internal/chat/service/chat_service.go` | ChatService — orquestrador com Strategy Pattern e detecção de intent |
| `internal/chat/service/chat_strategy_onboarding.go` | OnboardingStrategy — strategy para abertura de conta PJ |
| `internal/chat/handler/chat_handler.go` | Handler HTTP para `POST /v1/chat/{customerId}` |### Modificados

| Arquivo | O que mudou |
|---------|-------------|
| `internal/config/config.go` | Adicionado campo `ChatAgentURL` (env: `CHAT_AGENT_URL`) |
| `internal/handler/router.go` | Adicionado parâmetro `chatSvc` e rota `r.Post("/chat/{customerId}", ...)` |
| `cmd/bfa/main.go` | Wiring: ChatAgentClient → OnboardingStrategy → ChatService → Router |

---

## Jornada de Onboarding (Abertura de Conta)

A strategy de onboarding gerencia o fluxo conversacional de abertura de conta PJ.
São 3 etapas, que correspondem aos campos do `RegisterRequest`:

### Etapa 1 — Dados da Empresa

| Campo | Tipo | Exemplo |
|-------|------|---------|
| `cnpj` | string | `"12345678000190"` |
| `razaoSocial` | string | `"Empresa Exemplo LTDA"` |
| `nomeFantasia` | string | `"Empresa Exemplo"` |
| `email` | string | `"empresa@email.com"` |

### Etapa 2 — Dados do Representante Legal

| Campo | Tipo | Exemplo |
|-------|------|---------|
| `representanteName` | string | `"João Silva"` |
| `representanteCpf` | string | `"12345678901"` |
| `representantePhone` | string | `"+55 11 99999-0000"` |
| `representanteBirthDate` | string | `"1990-05-15"` |

### Etapa 3 — Senha

| Campo | Tipo | Exemplo |
|-------|------|---------|
| `password` | string | `"123456"` (6 dígitos numéricos) |

### State Machine (JourneyState)

```json
{
  "journey_type": "onboarding",
  "stage": 1,
  "status": "in_progress",
  "collected_data": {
    "cnpj": "12345678000190",
    "razaoSocial": "Empresa Exemplo LTDA"
  },
  "validation_errors": []
}
```

---

## Variáveis de Ambiente

| Variável | Default | Descrição |
|----------|---------|-----------|
| `CHAT_AGENT_URL` | `https://pj-assistant-agent-py-production.up.railway.app` | URL base do Agent Python |
| `AGENT_API_URL` | `http://localhost:8090` | URL do agent legado (POST /v1/agent/invoke) |

---

## Diferença entre as Rotas de Assistant

| Aspecto | POST /v1/assistant/{id} | POST /v1/chat/{id} |
|---------|------------------------|--------------------|
| **Método** | POST | POST |
| **Input** | `{"message": "...", "conversationId": "..."}` | `{"query": "..."}` |
| **O que faz** | Busca profile + transactions + chama agent | Strategy routing + chama agent |
| **Agent endpoint** | `POST /v1/agent/invoke` | `POST /v1/chat` |
| **Response** | Completa (tokens, tools, reasoning, profile) | Simples: `{"answer": "..."}` |
| **Uso** | Dashboard, análise completa | Chat, conversas rápidas |

---

## Como Adicionar uma Nova Strategy

Para adicionar suporte a um novo contexto (ex: PIX):

### 1. Criar o arquivo da strategy

```go
// internal/chat/service/chat_strategy_pix.go
package service

type PixStrategy struct {
    agentClient port.ChatAgentCaller
    logger      *zap.Logger
}

func (s *PixStrategy) CanHandle(intent string) bool {
    return intent == "pix"
}

func (s *PixStrategy) Handle(ctx context.Context, chatCtx *domain.ChatContext) (*domain.ChatResponse, error) {
    // Lógica específica de PIX aqui
    // Ex: validar se o cliente tem conta ativa, verificar limites, etc.
}
```

### 2. Registrar no main.go

```go
pixStrategy := service.NewPixStrategy(chatAgentClient, bankSvc, logger)
chatStrategies := []service.ChatStrategy{
    onboardingStrategy,  // intent "onboarding"
    pixStrategy,         // intent "pix"         ← NOVO
}
```

### 3. Adicionar keywords no detectIntent (chat_service.go)

As keywords de PIX já estão mapeadas! Basta criar a strategy.

---

## Fluxo Completo (Diagrama de Sequência)

```
Usuário          Frontend         BFA (Go)           Agent Python
  │                 │                │                    │
  │ "Quero abrir    │                │                    │
  │  uma conta PJ"  │                │                    │
  │ ───────────────→│                │                    │
  │                 │ POST /v1/chat/{id}                 │
  │                 │ {"query":"Quero abrir..."}          │
  │                 │ ──────────────→│                    │
  │                 │                │                    │
  │                 │                │ detectIntent()     │
  │                 │                │ → "onboarding"     │
  │                 │                │                    │
  │                 │                │ OnboardingStrategy │
  │                 │                │ .Handle()          │
  │                 │                │                    │
  │                 │                │ POST /v1/chat      │
  │                 │                │ {"query":"...",    │
  │                 │                │  "context":        │
  │                 │                │  "onboarding"}     │
  │                 │                │ ──────────────────→│
  │                 │                │                    │
  │                 │                │    {"answer":"..."}│
  │                 │                │ ←──────────────────│
  │                 │                │                    │
  │                 │ {"answer":"Olá! Vou te ajudar..."} │
  │                 │ ←──────────────│                    │
  │                 │                │                    │
  │ "Olá! Vou te   │                │                    │
  │  ajudar..."     │                │                    │
  │ ←───────────────│                │                    │
```

---

## Testando Localmente

```bash
# 1. Inicie o BFA
go run cmd/bfa/main.go

# 2. Teste a rota de chat
curl -s -X POST \
  http://localhost:8080/v1/chat/ab84533a-9589-41e1-b503-50cdc9cb9860 \
  -H "Content-Type: application/json" \
  -d '{"query": "Como abrir uma conta PJ?"}' | jq .

# 3. Teste com query genérica (vai pro fallback/default)
curl -s -X POST \
  http://localhost:8080/v1/chat/ab84533a-9589-41e1-b503-50cdc9cb9860 \
  -H "Content-Type: application/json" \
  -d '{"query": "Quais são as taxas do banco?"}' | jq .
```
```

---

*Tutorial criado como parte da Phase 21 do projeto PJ Assistant BFA.*
