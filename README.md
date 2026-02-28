# 🏦 PJ Assistant BFA — Backend Go

> API completa de **Banking as a Service (BaaS)** para clientes Pessoa Jurídica, construída com **Go**, **arquitetura hexagonal**, **Supabase (PostgREST)** como persistência e **Railway** para deploy contínuo.

**Stack:** Go 1.22 · Chi Router · Supabase · Prometheus · OpenTelemetry · Docker · Railway

---

## 📑 Índice

- [Visão Geral](#visão-geral)
- [Arquitetura](#arquitetura)
- [Estrutura de Pastas](#estrutura-de-pastas)
- [Camadas em Detalhe](#camadas-em-detalhe)
- [Endpoints da API](#endpoints-da-api)
- [Regras de Negócio](#regras-de-negócio)
- [Tabelas do Banco (Supabase)](#tabelas-do-banco-supabase)
- [Integrações Externas](#integrações-externas)
- [Variáveis de Ambiente](#variáveis-de-ambiente)
- [Como Rodar](#como-rodar)
- [Testes](#testes)
- [Deploy (Railway)](#deploy-railway)

---

## Visão Geral

O BFA é o backend de um aplicativo bancário PJ que oferece:

| Módulo | Descrição |
|--------|-----------|
| **Autenticação** | Registro, login, refresh token, reset de senha, JWT |
| **Contas** | Listagem de contas, saldo, extrato |
| **PIX** | Transferência (saldo ou cartão de crédito), agendamento, chaves, comprovante |
| **Cartão de Crédito** | Solicitar, bloquear/desbloquear/cancelar, fatura, pagamento de fatura |
| **Boletos** | Validação de código de barras, pagamento |
| **Débito** | Compra no débito |
| **Analytics** | Resumo financeiro, orçamentos, favoritos, limites, notificações |
| **Assistente IA** | Chat com agente LLM (RAG + tools) |
| **Dev Tools** | Endpoints para popular dados de teste |

**Princípio fundamental:** _Zero lógica no frontend — o backend retorna exatamente o que precisa ser exibido._

---

## Arquitetura

O projeto segue **Arquitetura Hexagonal** (Ports & Adapters), onde o domínio e os serviços não conhecem detalhes de infraestrutura.

```
┌─────────────────────────────────────────────────────────┐
│                    HTTP (chi router)                     │
│         handler/ — converte HTTP ↔ domain types         │
├─────────────────────────────────────────────────────────┤
│                     Service Layer                        │
│    service/ — orquestra regras de negócio, validações    │
├─────────────────────────────────────────────────────────┤
│               Domain (tipos puros Go)                    │
│  domain/ — structs, errors, sem dependência externa      │
├─────────────────────────────────────────────────────────┤
│                 Ports (interfaces)                        │
│  port/ — contratos que o service espera (ex: Store)      │
├─────────────────────────────────────────────────────────┤
│                Adapters (infra)                           │
│  infra/supabase/ — implementação concreta (PostgREST)    │
│  infra/client/   — HTTP clients para APIs externas       │
│  infra/cache/    — cache in-memory com TTL               │
│  infra/resilience/ — circuit breaker, retry, semaphore   │
│  infra/observability/ — logs (zap), métricas, tracing    │
└─────────────────────────────────────────────────────────┘
```

### Fluxo de uma Request

```
Client → HTTP Request
  → chi middleware (RequestID, RealIP, Logger, Tracing, Recoverer)
  → handler (decodifica JSON, chama service)
    → service (aplica regras de negócio)
      → port interface (contrato)
        → infra/supabase (implementação: PostgREST HTTP)
          → Supabase PostgreSQL
        ← resposta
      ← domain struct
    ← domain struct
  ← handler (codifica JSON)
← HTTP Response
```

### Injeção de Dependência

Tudo é construído no `main.go` (`cmd/bfa/main.go`):

```
Config → Logger → Tracer → Metrics → Cache → CircuitBreaker
  → SupabaseClient (implementa BankingStore + AuthStore + ProfileFetcher)
    → BankingService(store, metrics, logger)
    → AuthService(store, jwtSecret, ttls, devAuth, logger)
    → AssistantService(profileClient, txClient, agentClient, cache, metrics, logger)
      → Router(assistantSvc, bankSvc, authSvc, metrics, logger)
        → http.Server (graceful shutdown)
```

---

## Estrutura de Pastas

```
pj-assistant-bfa-go/
├── cmd/bfa/main.go              # Entrypoint — monta tudo e sobe o servidor
├── internal/
│   ├── config/config.go         # Leitura de variáveis de ambiente
│   ├── domain/                  # Tipos puros (sem dependência externa)
│   │   ├── account.go           # Account, Transaction, TransactionSummary
│   │   ├── analytics.go         # SpendingSummary, Budget, Favorite, Limit, Notification
│   │   ├── assistant.go         # AgentRequest/Response, AssistantRequest/Response
│   │   ├── auth.go              # Register, Login, Refresh, Password, Credentials
│   │   ├── billing.go           # BillPayment, BarcodeValidation, DebitPurchase
│   │   ├── cards.go             # CreditCard, Transaction, Invoice, API responses
│   │   ├── customer.go          # CustomerProfile, User, UserCompany
│   │   ├── devtools.go          # DevAddBalance, DevSetCreditLimit, DevGenerateTx
│   │   ├── errors.go            # 11 error types (NotFound, Validation, InsufficientFunds...)
│   │   ├── health.go            # HealthStatus, AgentMetrics, ListResponse[T]
│   │   └── pix.go               # PixKey, PixTransfer, PixReceipt, ScheduledTransfer
│   ├── port/                    # Interfaces (contratos)
│   │   ├── ports.go             # BankingStore (composto), AuthStore, ProfileFetcher, Cache
│   │   ├── account_port.go      # AccountStore
│   │   ├── cards_port.go        # CreditCardStore, CreditCardTransactionStore, CreditCardInvoiceStore
│   │   ├── pix_port.go          # PixKeyStore, PixTransferStore, PixReceiptStore, CustomerLookupStore
│   │   ├── billing_port.go      # BillingStore
│   │   └── analytics_port.go    # AnalyticsStore
│   ├── service/                 # Regras de negócio
│   │   ├── accounts_service.go
│   │   ├── analytics_service.go
│   │   ├── assistant.go         # AI Assistant (profile + transactions + agent)
│   │   ├── auth.go              # AuthService (JWT, bcrypt, dev_auth)
│   │   ├── auth_login.go
│   │   ├── auth_password.go
│   │   ├── auth_profile.go
│   │   ├── auth_registration.go
│   │   ├── auth_tokens.go
│   │   ├── billing_service.go
│   │   ├── cards_service.go     # Cartão de crédito + fatura + pagamento
│   │   ├── devtools_service.go
│   │   ├── pix_keys_service.go
│   │   ├── pix_receipts_service.go
│   │   ├── pix_transfer_service.go
│   │   └── scheduled_transfers_service.go
│   ├── handler/                 # HTTP handlers (chi)
│   │   ├── router.go            # Todas as rotas registradas aqui
│   │   ├── helpers.go           # writeJSON, writeError, handleServiceError
│   │   ├── middleware.go        # JWTAuthMiddleware
│   │   ├── accounts_handler.go
│   │   ├── analytics_handler.go
│   │   ├── assistant_handler.go
│   │   ├── auth_handler.go
│   │   ├── billing_handler.go
│   │   ├── cards_handler.go
│   │   ├── devtools_handler.go
│   │   ├── pix_keys_handler.go
│   │   ├── pix_receipts_handler.go
│   │   ├── pix_transfer_handler.go
│   │   └── scheduled_transfers_handler.go
│   └── infra/                   # Implementações concretas
│       ├── supabase/            # Adapter PostgREST
│       │   ├── client.go        # HTTP client base (doGet, doPost, doPatch, doDelete)
│       │   ├── helpers.go       # Funções auxiliares
│       │   ├── accounts_store.go
│       │   ├── analytics_store.go
│       │   ├── auth_store.go
│       │   ├── billing_store.go
│       │   ├── cards_store.go
│       │   ├── customer_lookup_store.go
│       │   ├── pix_keys_store.go
│       │   ├── pix_receipts_store.go
│       │   ├── pix_transfers_store.go
│       │   └── scheduled_transfers_store.go
│       ├── client/              # HTTP clients para APIs externas
│       ├── cache/               # Cache in-memory com TTL
│       ├── resilience/          # Circuit breaker (gobreaker), retry, semaphore
│       └── observability/       # Logger (zap), Metrics (Prometheus), Tracing (OTLP)
├── tests/integration/           # Testes de integração end-to-end
├── agent/                       # Agente IA em Python (RAG + LLM)
├── migrations/                  # SQL migrations para Supabase
├── Dockerfile                   # Multi-stage build (Go 1.22 → Alpine)
├── docker-compose.yml           # Ambiente local completo
├── Makefile                     # build, test, run, lint, docker
└── railway.toml                 # Configuração de deploy Railway
```

---

## Camadas em Detalhe

<details>
<summary><strong>🟦 Domain — Tipos puros</strong></summary>

A camada `domain/` define todos os structs e erros do sistema. **Não importa nenhum pacote externo** (exceto `time` da stdlib).

### Erros tipados

| Tipo | HTTP Status | Quando ocorre |
|------|-------------|---------------|
| `ErrNotFound` | 404 | Recurso não encontrado (conta, cartão, chave PIX...) |
| `ErrValidation` | 400 | Campo inválido ou ausente |
| `ErrInsufficientFunds` | 422 | Saldo insuficiente para a operação |
| `ErrLimitExceeded` | 422 | Limite de transação excedido |
| `ErrDuplicate` | 409 | Operação duplicada (idempotency key repetida) |
| `ErrForbidden` | 403 | Sem permissão para a ação |
| `ErrUnauthorized` | 401 | Credenciais inválidas ou token expirado |
| `ErrInvalidBarcode` | 400 | Código de barras/linha digitável inválido |
| `ErrExternalService` | 502 | Falha em serviço externo |
| `ErrTimeout` | 504 | Timeout de operação |
| `ErrCircuitOpen` | 503 | Circuit breaker aberto |
| `ErrConflict` | 409 | Conflito (ex: CNPJ já cadastrado) |
| `ErrAccountBlocked` | 403 | Conta bloqueada |
| `ErrInvalidCode` | 400 | Código de verificação inválido/expirado |

</details>

<details>
<summary><strong>🟩 Ports — Interfaces (contratos)</strong></summary>

Cada domínio define seu contrato em um arquivo separado em `port/`:

| Interface | Arquivo | Responsabilidade |
|-----------|---------|------------------|
| `AccountStore` | `account_port.go` | CRUD de contas, saldo |
| `PixKeyStore` | `pix_port.go` | Chaves PIX (listar, lookup, criar, deletar) |
| `PixTransferStore` | `pix_port.go` | Transferências PIX |
| `PixReceiptStore` | `pix_port.go` | Comprovantes PIX |
| `CustomerLookupStore` | `pix_port.go` | Busca nome/documento do customer |
| `ScheduledTransferStore` | `pix_port.go` | Agendamentos |
| `CreditCardStore` | `cards_port.go` | CRUD de cartões |
| `CreditCardTransactionStore` | `cards_port.go` | Transações de cartão |
| `CreditCardInvoiceStore` | `cards_port.go` | Faturas |
| `BillingStore` | `billing_port.go` | Boletos e compras no débito |
| `AnalyticsStore` | `analytics_port.go` | Analytics, budgets, favoritos, limites, notificações |
| **`BankingStore`** | `ports.go` | **Composto** — agrega TODAS as interfaces acima |
| `AuthStore` | `ports.go` | Autenticação (credentials, tokens, reset codes) |
| `ProfileFetcher` | `ports.go` | Busca perfil do customer |
| `TransactionsFetcher` | `ports.go` | Busca transações |
| `AgentCaller` | `ports.go` | Chama agente IA |

O `supabase.Client` implementa `BankingStore`, `AuthStore`, `ProfileFetcher` e `TransactionsFetcher` — tudo num único adapter.

</details>

<details>
<summary><strong>🟨 Service — Regras de negócio</strong></summary>

| Service | Construtor | Dependências |
|---------|-----------|--------------|
| `BankingService` | `NewBankingService(store, metrics, logger)` | `BankingStore` |
| `AuthService` | `NewAuthService(store, secret, accessTTL, refreshTTL, devAuth, logger)` | `AuthStore` |
| `Assistant` | `NewAssistant(profile, tx, agent, cache, metrics, logger)` | `ProfileFetcher`, `TransactionsFetcher`, `AgentCaller` |

### Constantes de negócio

| Constante | Valor | Onde | Significado |
|-----------|-------|------|-------------|
| `MinimumPaymentRate` | `0.15` (15%) | `cards_service.go` | % do total da fatura para pagamento mínimo |
| `DefaultTransactionPageSize` | `500` | `cards_service.go` | Máximo de transações buscadas por query |
| `PixCreditFeeRate` | `0.02` (2%) | `pix_transfer_handler.go` | Juros por parcela no PIX via cartão |
| `PixCreditMaxInstallments` | `12` | `pix_transfer_handler.go` | Máximo de parcelas no PIX via cartão |

</details>

<details>
<summary><strong>🟥 Handler — HTTP (chi router)</strong></summary>

Cada handler faz apenas:
1. Decodifica o JSON da request
2. Chama o service
3. Monta o response DTO (camelCase para o frontend)
4. Retorna JSON

**Middleware aplicado a todas as rotas:**

| Middleware | Função |
|------------|--------|
| `RequestID` | Gera ID único por request |
| `RealIP` | Resolve IP real (proxy) |
| `ZapLoggerMiddleware` | Log estruturado de cada request |
| `TracingMiddleware` | OpenTelemetry span |
| `Recoverer` | Recupera panics sem derrubar o server |
| `Heartbeat("/ping")` | Responde 200 em `/ping` |

**Middleware de autenticação:**

| Middleware | Rotas protegidas |
|------------|-----------------|
| `JWTAuthMiddleware` | `POST /v1/auth/logout`, `PUT /v1/auth/password`, `PUT /v1/customers/{id}/profile`, `PUT /v1/customers/{id}/representative` |

</details>

<details>
<summary><strong>🟪 Infra — Implementações concretas</strong></summary>

### Supabase (PostgREST)

O `supabase.Client` faz chamadas HTTP ao PostgREST do Supabase:

| Método | Verbo HTTP | Uso |
|--------|-----------|-----|
| `doGet(ctx, table, query)` | GET | Leitura |
| `doPost(ctx, table, body)` | POST | Criação (com `Prefer: return=representation`) |
| `doPatch(ctx, table, query, body)` | PATCH | Atualização |
| `doDelete(ctx, table, query)` | DELETE | Deleção |

Autenticação via headers: `apikey` + `Authorization: Bearer <service_role_key>`

### Cache

Cache in-memory genérico com TTL configurável. Usado para cachear perfis de clientes no `AssistantService`.

### Resilience

| Componente | Lib | Função |
|------------|-----|--------|
| Circuit Breaker | `sony/gobreaker` | Protege contra falhas em cascata |
| Retry com Backoff | Custom | Retenta chamadas com exponential backoff |
| Semaphore | Custom | Limita concorrência máxima |

### Observability

| Componente | Lib | Função |
|------------|-----|--------|
| Logger | `zap` | Log estruturado JSON |
| Métricas | `prometheus/client_golang` | Histogramas, counters (request duration, errors, cache, tokens) |
| Tracing | `opentelemetry` | Distributed tracing (OTLP/gRPC) |

Cada instância de `Metrics` usa seu próprio `prometheus.Registry` (evita panic de registro duplicado em testes).

</details>

---

## Endpoints da API

<details>
<summary><strong>🔐 Autenticação</strong></summary>

| Método | Rota | Descrição | Auth |
|--------|------|-----------|------|
| `POST` | `/v1/auth/register` | Cadastro de empresa PJ | ❌ |
| `POST` | `/v1/auth/login` | Login (CPF + senha) | ❌ |
| `POST` | `/v1/auth/refresh` | Renovar access token | ❌ |
| `POST` | `/v1/auth/logout` | Revogar tokens | ✅ JWT |
| `POST` | `/v1/auth/password/reset-request` | Solicitar reset de senha | ❌ |
| `POST` | `/v1/auth/password/reset-confirm` | Confirmar reset com código | ❌ |
| `PUT` | `/v1/auth/password` | Alterar senha (logado) | ✅ JWT |

</details>

<details>
<summary><strong>👤 Cliente / Perfil</strong></summary>

| Método | Rota | Descrição | Auth |
|--------|------|-----------|------|
| `GET` | `/v1/customers/{customerId}/profile` | Dados do perfil PJ | ❌ |
| `PUT` | `/v1/customers/{customerId}/profile` | Atualizar perfil | ✅ JWT |
| `PUT` | `/v1/customers/{customerId}/representative` | Atualizar representante legal | ✅ JWT |

</details>

<details>
<summary><strong>🏦 Contas</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/v1/customers/{customerId}/accounts` | Listar contas |
| `GET` | `/v1/customers/{customerId}/accounts/{accountId}` | Detalhes de uma conta |
| `GET` | `/v1/customers/{customerId}/accounts/{accountId}/balance` | Saldo da conta |

</details>

<details>
<summary><strong>📊 Transações / Extrato</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/v1/customers/{customerId}/transactions` | Extrato (últimas 500 transações) |
| `GET` | `/v1/customers/{customerId}/transactions/summary` | Resumo (créditos, débitos, saldo, top categorias) |

</details>

<details>
<summary><strong>💸 PIX</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/v1/pix/keys/lookup` | Consultar chave PIX (busca destinatário) |
| `GET` | `/v1/pix/lookup` | Alias para lookup |
| `POST` | `/v1/pix/transfer` | Transferência PIX (saldo) |
| `POST` | `/v1/pix/credit-card` | PIX via cartão de crédito (com juros + parcelas) |
| `POST` | `/v1/pix/credit` | Alias para PIX crédito |
| `POST` | `/v1/pix/schedule` | Agendar transferência PIX |
| `DELETE` | `/v1/pix/schedule/{scheduleId}` | Cancelar agendamento |
| `GET` | `/v1/customers/{customerId}/pix/scheduled` | Listar agendamentos |
| `GET` | `/v1/pix/scheduled/{customerId}` | Alias para listar agendamentos |
| `POST` | `/v1/pix/keys/register` | Registrar nova chave PIX |
| `DELETE` | `/v1/pix/keys` | Deletar chave PIX por valor |
| `GET` | `/v1/customers/{customerId}/pix/keys` | Listar chaves PIX |
| `DELETE` | `/v1/customers/{customerId}/pix/keys/{keyId}` | Deletar chave PIX por ID |
| `GET` | `/v1/pix/receipts/{receiptId}` | Comprovante PIX por ID |
| `GET` | `/v1/pix/transfers/{transferId}/receipt` | Comprovante PIX por transferência |
| `GET` | `/v1/customers/{customerId}/pix/receipts` | Listar comprovantes PIX |

</details>

<details>
<summary><strong>💳 Cartão de Crédito</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/v1/customers/{customerId}/cards` | Listar cartões |
| `GET` | `/v1/customers/{customerId}/credit-cards` | Alias |
| `GET` | `/v1/customers/{customerId}/credit-limit` | Consultar limite de crédito |
| `POST` | `/v1/cards/request` | Solicitar novo cartão |
| `POST` | `/v1/customers/{customerId}/credit-cards/request` | Alias |
| `GET` | `/v1/cards/{cardId}/invoices/{month}` | Fatura por mês (YYYY-MM) |
| `GET` | `/v1/customers/{customerId}/credit-cards/{cardId}/invoice` | Fatura do mês atual |
| `POST` | `/v1/customers/{customerId}/credit-cards/{cardId}/invoice/pay` | Pagar fatura |
| `POST` | `/v1/cards/{cardId}/block` | Bloquear cartão |
| `POST` | `/v1/cards/{cardId}/unblock` | Desbloquear cartão |
| `POST` | `/v1/cards/{cardId}/cancel` | Cancelar cartão |

</details>

<details>
<summary><strong>📄 Boletos</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `POST` | `/v1/bills/validate` | Validar código de barras |
| `POST` | `/v1/bills/pay` | Pagar boleto |
| `GET` | `/v1/customers/{customerId}/bills/history` | Histórico de boletos pagos |

</details>

<details>
<summary><strong>📈 Analytics / Financeiro</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/v1/customers/{customerId}/financial/summary` | Resumo financeiro completo |
| `POST` | `/v1/debit/purchase` | Compra no débito |
| `GET` | `/v1/customers/{customerId}/analytics/budgets` | Listar orçamentos |
| `POST` | `/v1/customers/{customerId}/analytics/budgets` | Criar orçamento |
| `PUT` | `/v1/customers/{customerId}/analytics/budgets/{budgetId}` | Atualizar orçamento |
| `GET` | `/v1/customers/{customerId}/favorites` | Listar favoritos |
| `POST` | `/v1/customers/{customerId}/favorites` | Criar favorito |
| `DELETE` | `/v1/customers/{customerId}/favorites/{favoriteId}` | Remover favorito |
| `GET` | `/v1/customers/{customerId}/limits` | Listar limites |
| `PUT` | `/v1/customers/{customerId}/limits/{limitType}` | Atualizar limite |
| `GET` | `/v1/customers/{customerId}/notifications` | Listar notificações |
| `POST` | `/v1/customers/{customerId}/notifications/{notifId}/read` | Marcar notificação como lida |

</details>

<details>
<summary><strong>🤖 Assistente IA</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `POST` | `/v1/assistant/{customerId}` | Pergunta ao assistente |
| `POST` | `/v1/chat` | Alias para o assistente |

O assistente busca perfil + transações em paralelo (errgroup), envia ao agente IA e retorna resposta com metadata (tokens, fontes RAG, ferramentas usadas).

</details>

<details>
<summary><strong>🔧 Dev Tools</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `POST` | `/v1/dev/add-balance` | Adicionar saldo à conta |
| `POST` | `/v1/dev/set-credit-limit` | Definir limite do cartão |
| `POST` | `/v1/dev/generate-transactions` | Gerar transações aleatórias no extrato |
| `POST` | `/v1/dev/add-card-purchase` | Adicionar compra no cartão de crédito |
| `POST` | `/v1/dev/card-purchase` | Alias |

</details>

<details>
<summary><strong>⚙️ Operacional</strong></summary>

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/healthz` | Health check (verifica Supabase) |
| `GET` | `/readyz` | Readiness probe |
| `GET` | `/ping` | Heartbeat |
| `GET` | `/metrics` | Métricas Prometheus |
| `GET` | `/v1/metrics/agent` | Métricas do agente IA (tokens, latência, custo) |

</details>

---

## Regras de Negócio

<details>
<summary><strong>💸 PIX — Transferência via Saldo</strong></summary>

1. Busca a conta primária do customer
2. Valida saldo disponível ≥ valor (`ErrInsufficientFunds` se não)
3. Cria o registro `pix_transfers` com `funded_by = "balance"`
4. Debita o saldo da conta (`UpdateAccountBalance`)
5. Cria transação no extrato (`transactions`) tipo `pix_sent`
6. Cria comprovante (`pix_receipts`) com dados do remetente e destinatário
7. Retorna `transactionId`, `receiptId`, `newBalance`, `e2eId`

</details>

<details>
<summary><strong>💳 PIX — Transferência via Cartão de Crédito</strong></summary>

1. Valida `installments` entre 1 e 12
2. Calcula juros: `totalWithFees = amount × (1 + 0.02 × (installments - 1))`
   - Exemplo: R$ 1.000 em 3x → R$ 1.000 × 1.04 = R$ 1.040
3. Valida limite disponível do cartão ≥ `totalWithFees`
4. Cria `pix_transfers` com `funded_by = "credit_card"`
5. **NÃO** debita o saldo da conta (débito fica no cartão)
6. Atualiza `used_limit` e `available_limit` do cartão
7. Atualiza `pix_credit_used` do cartão
8. Cria transação no **cartão de crédito** (`credit_card_transactions`) — não no extrato
9. Cria comprovante PIX
10. **Comprovante** mostra apenas o `amount` (valor do PIX enviado)
11. **Fatura** mostra breakdown completo: `originalAmount`, `feeAmount`, `totalWithFees`, `installmentAmount`

</details>

<details>
<summary><strong>📋 Fatura do Cartão</strong></summary>

1. Busca ou cria automaticamente a fatura do mês (`GetCardInvoiceByMonth`)
2. Se não existe → cria com datas calculadas a partir de `billingDay` e `dueDay`
3. Busca todas as transações do cartão no período (`DefaultTransactionPageSize = 500`)
4. Filtra transações que pertencem ao mês da fatura
5. Recalcula `totalAmount` = soma de todas as transações do mês
6. Calcula `minimumPayment` = `totalAmount × MinimumPaymentRate` (15%)
7. Atualiza `totalAmount` e `minimumPayment` no banco
8. Retorna fatura com lista de transações (ordenadas por data desc)

</details>

<details>
<summary><strong>💰 Pagamento de Fatura</strong></summary>

1. Busca a fatura do mês atual
2. Valida tipo de pagamento:
   - `total` → paga o `totalAmount` completo
   - `minimum` → paga o `minimumPayment` (15%)
   - `custom` → paga valor informado (deve ser ≥ `minimumPayment`)
3. Debita o saldo da conta
4. Atualiza `paid_amount` da fatura
5. Se `paidAmount ≥ totalAmount` → status `paid`, senão `partial`
6. Libera limite do cartão (`availableLimit += paidAmount`)
7. Cria transação no extrato tipo `credit_card_payment`

</details>

<details>
<summary><strong>🔑 Registro (Cadastro PJ)</strong></summary>

1. Valida CNPJ (14 dígitos numéricos)
2. Verifica se CNPJ já existe (`ErrConflict`)
3. Hash da senha com bcrypt (ou plain-text se `DEV_AUTH=true`)
4. Cria `customer_profiles` com dados da empresa + representante
5. Cria `accounts` (conta corrente com saldo 0)
6. Cria `auth_credentials` (hash da senha)
7. Se `DEV_AUTH=true`, cria `dev_logins` (CPF + senha em plain-text)
8. Registra chave PIX automática (CNPJ)
9. Retorna `customerId`, `agencia`, `conta`

</details>

<details>
<summary><strong>🔐 Login</strong></summary>

1. Busca customer pelo CPF do representante
2. Verifica se a conta está bloqueada (`locked_until`)
3. Se `DEV_AUTH=true` → busca `dev_logins` (plain-text) como fallback
4. Se `DEV_AUTH=false` → compara bcrypt hash
5. Se falhar → incrementa `failed_attempts` (bloqueia após 5 tentativas, por 15 min)
6. Se sucesso → zera `failed_attempts`, gera JWT access token + refresh token
7. Retorna `accessToken`, `refreshToken`, `expiresIn`, `customerId`, `customerName`

</details>

<details>
<summary><strong>💳 Solicitar Cartão</strong></summary>

1. Valida `account_id` (deve existir)
2. Aplica defaults: brand=Visa, type=corporate, billingDay=10, dueDay=20, limit=10000
3. Busca o nome real do customer (`GetCustomerName`) para `card_holder_name`
4. Gera últimos 4 dígitos aleatórios (`UnixNano % 10000`)
5. Cria o cartão com status `active`, `pix_credit_enabled = true`
6. Retorna cartão com campos `cardType`, `holderName`, `brand`, `lastFourDigits`

</details>

<details>
<summary><strong>📄 Pagamento de Boleto</strong></summary>

1. Valida código de barras (44 dígitos) ou linha digitável (47-48 dígitos)
2. Extrai dados: tipo, banco, valor, vencimento, beneficiário
3. Debita saldo da conta
4. Cria registro `bill_payments`
5. Cria transação no extrato tipo `bill_payment`

</details>

---

## Tabelas do Banco (Supabase)

<details>
<summary><strong>👤 customer_profiles</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | Igual ao `customer_id` |
| `customer_id` | UUID | ID único do cliente PJ |
| `name` | TEXT | Razão social / nome fantasia |
| `document` | TEXT | CNPJ (14 dígitos) |
| `company_name` | TEXT | Nome fantasia |
| `email` | TEXT | Email da empresa |
| `segment` | TEXT | Segmento (pj_standard, middle_market...) |
| `monthly_revenue` | NUMERIC | Faturamento mensal |
| `account_age_months` | INT | Idade da conta em meses |
| `credit_score` | INT | Score de crédito (0-1000) |
| `account_status` | TEXT | active, blocked, suspended |
| `relationship_since` | TIMESTAMP | Data de início do relacionamento |
| `representante_name` | TEXT | Nome do representante legal |
| `representante_cpf` | TEXT | CPF do representante |
| `representante_phone` | TEXT | Telefone do representante |
| `representante_birth_date` | TEXT | Data de nascimento do representante |
| `created_at` | TIMESTAMP | Criação do registro |
| `updated_at` | TIMESTAMP | Última atualização |

</details>

<details>
<summary><strong>🏦 accounts</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID da conta |
| `customer_id` | UUID (FK) | Dono da conta |
| `account_type` | TEXT | checking, savings |
| `branch` | TEXT | Agência (4 dígitos) |
| `account_number` | TEXT | Número da conta |
| `digit` | TEXT | Dígito verificador |
| `bank_code` | TEXT | Código do banco (341 = Itaú) |
| `bank_name` | TEXT | Nome do banco |
| `balance` | NUMERIC | Saldo atual |
| `available_balance` | NUMERIC | Saldo disponível |
| `overdraft_limit` | NUMERIC | Limite de cheque especial |
| `currency` | TEXT | Moeda (BRL) |
| `status` | TEXT | active, blocked |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>📊 transactions</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID da transação |
| `customer_id` | UUID (FK) | Cliente |
| `date` | TIMESTAMP | Data da transação |
| `amount` | NUMERIC | Valor (negativo = débito) |
| `type` | TEXT | pix_sent, pix_received, debit_purchase, credit_purchase, bill_payment, transfer_in, transfer_out, credit_card_payment |
| `category` | TEXT | Categoria (revenue, supplier, utilities, salary, other...) |
| `description` | TEXT | Descrição legível |
| `counterparty` | TEXT | Nome da contraparte |

</details>

<details>
<summary><strong>🔑 pix_keys</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID da chave |
| `account_id` | UUID (FK) | Conta associada |
| `customer_id` | UUID (FK) | Cliente |
| `key_type` | TEXT | cpf, cnpj, email, phone, random |
| `key_value` | TEXT | Valor da chave |
| `status` | TEXT | active, inactive |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>💸 pix_transfers</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID da transferência |
| `idempotency_key` | TEXT (UNIQUE) | Chave de idempotência |
| `source_account_id` | UUID (FK) | Conta de origem |
| `source_customer_id` | UUID (FK) | Cliente de origem |
| `destination_key_type` | TEXT | Tipo da chave destino |
| `destination_key_value` | TEXT | Valor da chave destino |
| `destination_name` | TEXT | Nome do destinatário |
| `destination_document` | TEXT | Documento do destinatário |
| `amount` | NUMERIC | Valor do PIX |
| `description` | TEXT | Descrição |
| `status` | TEXT | completed, pending, failed |
| `failure_reason` | TEXT | Motivo da falha |
| `end_to_end_id` | TEXT | ID end-to-end (E2E) |
| `funded_by` | TEXT | balance ou credit_card |
| `credit_card_id` | UUID | Cartão usado (se credit_card) |
| `credit_card_installments` | INT | Parcelas (se credit_card) |
| `scheduled_for` | TIMESTAMP | Data agendada (null = imediato) |
| `executed_at` | TIMESTAMP | Data de execução |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>🧾 pix_receipts</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID do comprovante |
| `transfer_id` | UUID (FK) | Transferência associada |
| `customer_id` | UUID (FK) | Cliente |
| `direction` | TEXT | sent ou received |
| `amount` | NUMERIC | Valor do PIX |
| `original_amount` | NUMERIC | Valor original (antes dos juros) |
| `fee_amount` | NUMERIC | Valor dos juros |
| `total_amount` | NUMERIC | Total com juros |
| `description` | TEXT | Descrição |
| `end_to_end_id` | TEXT | ID E2E |
| `funded_by` | TEXT | balance ou credit_card |
| `installments` | INT | Parcelas |
| `sender_name` | TEXT | Nome do remetente |
| `sender_document` | TEXT | Documento do remetente |
| `sender_bank` | TEXT | Banco do remetente |
| `sender_branch` | TEXT | Agência do remetente |
| `sender_account` | TEXT | Conta do remetente |
| `recipient_name` | TEXT | Nome do destinatário |
| `recipient_document` | TEXT | Documento do destinatário |
| `recipient_bank` | TEXT | Banco do destinatário |
| `recipient_branch` | TEXT | Agência do destinatário |
| `recipient_account` | TEXT | Conta do destinatário |
| `recipient_key_type` | TEXT | Tipo da chave PIX |
| `recipient_key_value` | TEXT | Valor da chave PIX |
| `transaction_id` | TEXT | ID da transação (cartão) |
| `status` | TEXT | completed |
| `executed_at` | TEXT | Data de execução (ISO8601) |
| `created_at` | TEXT | Data de criação (ISO8601) |

</details>

<details>
<summary><strong>⏰ scheduled_transfers</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID do agendamento |
| `idempotency_key` | TEXT | Chave de idempotência |
| `source_account_id` | UUID (FK) | Conta de origem |
| `source_customer_id` | UUID (FK) | Cliente |
| `transfer_type` | TEXT | pix, ted, doc, internal |
| `destination_bank_code` | TEXT | Código do banco destino |
| `destination_branch` | TEXT | Agência destino |
| `destination_account` | TEXT | Conta destino |
| `destination_account_type` | TEXT | Tipo da conta |
| `destination_name` | TEXT | Nome do destinatário |
| `destination_document` | TEXT | Documento do destinatário |
| `amount` | NUMERIC | Valor |
| `description` | TEXT | Descrição |
| `schedule_type` | TEXT | once, daily, weekly, biweekly, monthly |
| `scheduled_date` | TEXT | Data agendada (YYYY-MM-DD) |
| `next_execution_date` | TEXT | Próxima execução |
| `recurrence_count` | INT | Quantas vezes já executou |
| `max_recurrences` | INT | Máximo de recorrências |
| `status` | TEXT | active, completed, cancelled |
| `failure_reason` | TEXT | Motivo da falha |
| `last_executed_at` | TIMESTAMP | Última execução |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>💳 credit_cards</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID do cartão |
| `customer_id` | UUID (FK) | Cliente |
| `account_id` | UUID (FK) | Conta vinculada |
| `card_number_last4` | TEXT | Últimos 4 dígitos |
| `card_holder_name` | TEXT | Nome do titular (busca do perfil) |
| `card_brand` | TEXT | Visa, Mastercard, Elo, Amex |
| `card_type` | TEXT | corporate, virtual, additional |
| `credit_limit` | NUMERIC | Limite total |
| `available_limit` | NUMERIC | Limite disponível |
| `used_limit` | NUMERIC | Limite consumido |
| `billing_day` | INT | Dia de fechamento (1-28) |
| `due_day` | INT | Dia de vencimento (1-28) |
| `status` | TEXT | active, blocked, cancelled |
| `pix_credit_enabled` | BOOL | PIX via crédito habilitado |
| `pix_credit_limit` | NUMERIC | Limite para PIX crédito |
| `pix_credit_used` | NUMERIC | Quanto já usou de PIX crédito |
| `is_contactless_enabled` | BOOL | Pagamento por aproximação |
| `is_international_enabled` | BOOL | Compras internacionais |
| `is_online_enabled` | BOOL | Compras online |
| `daily_limit` | NUMERIC | Limite diário |
| `single_transaction_limit` | NUMERIC | Limite por transação |
| `issued_at` | TIMESTAMP | Data de emissão |
| `expires_at` | TIMESTAMP | Data de validade |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>🧾 credit_card_transactions</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID da transação |
| `card_id` | UUID (FK) | Cartão |
| `customer_id` | UUID (FK) | Cliente |
| `transaction_date` | TIMESTAMP | Data da compra |
| `amount` | NUMERIC | Valor da parcela |
| `original_amount` | NUMERIC | Valor total original (PIX crédito) |
| `installment_amount` | NUMERIC | Valor por parcela |
| `merchant_name` | TEXT | Nome do estabelecimento |
| `category` | TEXT | Categoria |
| `installments` | INT | Total de parcelas |
| `current_installment` | INT | Parcela atual (ex: 2 de 3) |
| `transaction_type` | TEXT | purchase, pix_credit, refund, fee |
| `status` | TEXT | posted, pending, reversed |
| `description` | TEXT | Descrição legível |
| `is_international` | BOOL | Compra internacional |

</details>

<details>
<summary><strong>📋 credit_card_invoices</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID da fatura |
| `card_id` | UUID (FK) | Cartão |
| `customer_id` | UUID (FK) | Cliente |
| `reference_month` | TEXT | Mês de referência (YYYY-MM) |
| `open_date` | TEXT | Data de abertura |
| `close_date` | TEXT | Data de fechamento |
| `due_date` | TEXT | Data de vencimento |
| `total_amount` | NUMERIC | Valor total (recalculado dinamicamente) |
| `minimum_payment` | NUMERIC | Pagamento mínimo (15% do total) |
| `interest_amount` | NUMERIC | Juros |
| `status` | TEXT | open, closed, paid, partial, overdue |
| `paid_amount` | NUMERIC | Valor já pago |
| `barcode` | TEXT | Código de barras do boleto |
| `digitable_line` | TEXT | Linha digitável |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>📄 bill_payments</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID do pagamento |
| `idempotency_key` | TEXT | Chave de idempotência |
| `customer_id` | UUID (FK) | Cliente |
| `account_id` | UUID (FK) | Conta debitada |
| `input_method` | TEXT | typed, pasted, camera_scan, file_upload |
| `barcode` | TEXT | Código de barras (44 dígitos) |
| `digitable_line` | TEXT | Linha digitável (47-48 dígitos) |
| `bill_type` | TEXT | bank_slip, utility, tax_slip, government |
| `beneficiary_name` | TEXT | Nome do beneficiário |
| `beneficiary_document` | TEXT | Documento do beneficiário |
| `original_amount` | NUMERIC | Valor original |
| `final_amount` | NUMERIC | Valor pago |
| `due_date` | TEXT | Vencimento |
| `payment_date` | TEXT | Data do pagamento |
| `scheduled_date` | TEXT | Data agendada |
| `status` | TEXT | completed, pending, failed |
| `failure_reason` | TEXT | Motivo da falha |
| `receipt_url` | TEXT | URL do comprovante |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>📊 spending_summaries</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK) | Cliente |
| `period_type` | TEXT | daily, weekly, monthly, yearly |
| `period_start` | TEXT | Início do período (YYYY-MM-DD) |
| `period_end` | TEXT | Fim do período |
| `total_income` | NUMERIC | Total de receitas |
| `total_expenses` | NUMERIC | Total de despesas |
| `net_cashflow` | NUMERIC | Fluxo de caixa líquido |
| `transaction_count` | INT | Total de transações |
| `category_breakdown` | JSONB | Gastos por categoria |
| `pix_sent_total` | NUMERIC | Total PIX enviado |
| `pix_received_total` | NUMERIC | Total PIX recebido |
| `credit_card_total` | NUMERIC | Total gasto no cartão |
| `bills_paid_total` | NUMERIC | Total de boletos pagos |

</details>

<details>
<summary><strong>🔐 auth_credentials</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK, UNIQUE) | Cliente |
| `password_hash` | TEXT | Hash bcrypt da senha |
| `failed_attempts` | INT | Tentativas de login falhadas |
| `locked_until` | TIMESTAMP | Bloqueado até (null = desbloqueado) |
| `last_login_at` | TIMESTAMP | Último login bem-sucedido |
| `password_changed_at` | TIMESTAMP | Última troca de senha |

</details>

<details>
<summary><strong>🔄 auth_refresh_tokens</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK) | Cliente |
| `token_hash` | TEXT | Hash SHA-256 do refresh token |
| `expires_at` | TIMESTAMP | Expiração |
| `revoked` | BOOL | Se foi revogado |

</details>

<details>
<summary><strong>🔑 auth_password_reset_codes</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK) | Cliente |
| `code` | TEXT | Código de 6 dígitos |
| `expires_at` | TIMESTAMP | Expiração (15 minutos) |
| `used` | BOOL | Se já foi utilizado |

</details>

<details>
<summary><strong>🧪 dev_logins (DEV_AUTH only)</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK) | Cliente |
| `cpf` | TEXT | CPF em plain-text |
| `password` | TEXT | Senha em plain-text |

Essa tabela só é usada quando `DEV_AUTH=true`. **Nunca** usar em produção.

</details>

<details>
<summary><strong>⭐ favorites</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK) | Cliente |
| `user_id` | UUID | Usuário |
| `nickname` | TEXT | Apelido do contato |
| `destination_type` | TEXT | pix, ted, doc, bill |
| `pix_key_type` | TEXT | Tipo da chave PIX |
| `pix_key_value` | TEXT | Valor da chave PIX |
| `recipient_name` | TEXT | Nome do destinatário |
| `recipient_document` | TEXT | Documento |
| `usage_count` | INT | Vezes utilizado |
| `last_used_at` | TIMESTAMP | Último uso |

</details>

<details>
<summary><strong>📏 transaction_limits</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK) | Cliente |
| `transaction_type` | TEXT | pix, ted, debit, credit_card, bill_payment |
| `daily_limit` | NUMERIC | Limite diário |
| `daily_used` | NUMERIC | Quanto já usou hoje |
| `monthly_limit` | NUMERIC | Limite mensal |
| `monthly_used` | NUMERIC | Quanto já usou no mês |
| `single_limit` | NUMERIC | Limite por transação |
| `nightly_single_limit` | NUMERIC | Limite noturno por transação |
| `nightly_daily_limit` | NUMERIC | Limite noturno diário |

</details>

<details>
<summary><strong>🔔 notifications</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `user_id` | UUID | Usuário |
| `customer_id` | UUID | Cliente |
| `type` | TEXT | Tipo da notificação |
| `title` | TEXT | Título |
| `body` | TEXT | Corpo da mensagem |
| `channel` | TEXT | Canal (push, in_app, email) |
| `priority` | TEXT | Prioridade (high, medium, low) |
| `is_read` | BOOL | Se foi lida |
| `read_at` | TIMESTAMP | Quando foi lida |
| `created_at` | TIMESTAMP | Criação |

</details>

<details>
<summary><strong>💰 spending_budgets</strong></summary>

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `id` | UUID (PK) | ID |
| `customer_id` | UUID (FK) | Cliente |
| `category` | TEXT | Categoria (supplier, utilities, salary...) |
| `monthly_limit` | NUMERIC | Limite mensal para a categoria |
| `alert_threshold_pct` | NUMERIC | % para alertar (ex: 0.80 = 80%) |
| `is_active` | BOOL | Se o orçamento está ativo |

</details>

---

## Integrações Externas

<details>
<summary><strong>🗄️ Supabase (PostgREST)</strong></summary>

- **Função:** Persistência de todos os dados
- **Protocolo:** HTTP REST (PostgREST)
- **Autenticação:** `apikey` + `Authorization: Bearer <service_role_key>`
- **Headers especiais:**
  - `Prefer: return=representation` — retorna o registro criado/atualizado
  - `Prefer: count=exact` — retorna contagem exata
- **URL:** `https://<project>.supabase.co/rest/v1/<table>?<filters>`
- **Filtros PostgREST:** `customer_id=eq.<uuid>`, `status=eq.active`, `order=created_at.desc`

</details>

<details>
<summary><strong>🤖 Agente IA (Python)</strong></summary>

- **Função:** Responde perguntas do cliente sobre finanças
- **Protocolo:** HTTP POST
- **URL:** Configurável via `AGENT_API_URL` (default: `http://localhost:8090`)
- **Input:** `AgentRequest` (perfil + transações + query)
- **Output:** `AgentResponse` (answer, reasoning, confidence, tokens, sources, tools)
- **Pipeline:** RAG (ChromaDB) + LLM (GPT-4o) + Financial Analysis Tools

</details>

<details>
<summary><strong>📊 Prometheus</strong></summary>

- **Função:** Coleta de métricas
- **Métricas registradas:**
  - `bfa_request_duration_seconds` — latência por operação (histogram)
  - `bfa_external_errors_total` — erros de serviços externos (counter)
  - `bfa_cache_hits_total` — cache hits (counter)
  - `bfa_cache_misses_total` — cache misses (counter)
  - `bfa_llm_tokens_total` — tokens LLM consumidos (counter)
  - `bfa_requests_total` — total de requests por status (counter)
- **Endpoint:** `GET /metrics`

</details>

<details>
<summary><strong>🔭 OpenTelemetry</strong></summary>

- **Função:** Distributed tracing
- **Protocolo:** OTLP/gRPC
- **Endpoint:** Configurável via `OTEL_EXPORTER_OTLP_ENDPOINT` (default: `localhost:4317`)
- **Service name:** `pj-assistant-bfa`
- **Spans:** Criados em cada handler e service method

</details>

<details>
<summary><strong>🚂 Railway</strong></summary>

- **Função:** Deploy contínuo (auto-deploy no push para `main`)
- **Builder:** Dockerfile (multi-stage)
- **Health check:** `GET /healthz` (timeout 10s)
- **Restart policy:** On failure (max 5 retries)
- **URL produção:** `https://pj-assistant-bfa-go-production.up.railway.app`

</details>

---

## Variáveis de Ambiente

| Variável | Default | Descrição |
|----------|---------|-----------|
| `PORT` | `8080` | Porta do servidor |
| `LOG_LEVEL` | `info` | Nível de log (debug, info, warn, error) |
| `SUPABASE_URL` | — | URL do projeto Supabase |
| `SUPABASE_ANON_KEY` | — | Chave pública do Supabase |
| `SUPABASE_SERVICE_ROLE_KEY` | — | Chave de serviço do Supabase (full access) |
| `USE_SUPABASE` | `true` | Se usa Supabase como backend de dados |
| `PROFILE_API_URL` | `http://localhost:8081` | URL da API de perfil (se não usar Supabase) |
| `TRANSACTIONS_API_URL` | `http://localhost:8082` | URL da API de transações (se não usar Supabase) |
| `AGENT_API_URL` | `http://localhost:8090` | URL do agente IA (Python) |
| `HTTP_TIMEOUT` | `10s` | Timeout para chamadas HTTP |
| `MAX_RETRIES` | `3` | Máximo de retentativas |
| `INITIAL_BACKOFF` | `100ms` | Backoff inicial entre retentativas |
| `MAX_CONCURRENCY` | `50` | Máximo de requisições concorrentes |
| `CACHE_TTL` | `5m` | TTL do cache de perfis |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | Endpoint do collector OTLP |
| `JWT_SECRET` | `bfa-default-dev-secret-change-me` | Secret para assinar JWTs |
| `JWT_ACCESS_TTL` | `15m` | Duração do access token |
| `JWT_REFRESH_TTL` | `168h` (7 dias) | Duração do refresh token |
| `DEV_AUTH` | `false` | Habilita login plain-text (dev_logins) |

---

## Como Rodar

### Pré-requisitos

- Go 1.22+
- Conta Supabase com tabelas criadas
- (Opcional) Docker para ambiente completo

### Localmente

```bash
# 1. Clone o repositório
git clone https://github.com/Boddenberg/pj-assistant-bfa-go.git
cd pj-assistant-bfa-go

# 2. Configure o .env
cp .env.example .env
# Edite com suas credenciais Supabase

# 3. Rode
make run
# ou
go run ./cmd/bfa
```

### Com Docker

```bash
docker compose up --build -d
```

### Makefile

```bash
make build          # Compila o binário
make run            # Compila e roda
make test           # Roda testes Go
make test-cover     # Testes com cobertura HTML
make lint           # golangci-lint
make docker-up      # Sobe tudo com Docker Compose
make docker-down    # Para tudo
make test-all       # Testes Go + Python
make clean          # Limpa artefatos
```

---

## Testes

```bash
# Testes unitários + integração
go test ./... -v -race

# Com cobertura
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

| Pacote | O que testa |
|--------|-------------|
| `internal/handler` | Handlers HTTP (healthz, readyz, metrics) |
| `internal/service` | AssistantService (mocks de profile, transactions, agent) |
| `internal/infra/cache` | Cache in-memory com TTL |
| `internal/infra/resilience` | Circuit breaker, retry |
| `tests/integration` | Fluxo completo end-to-end com mock servers |

---

## Deploy (Railway)

O deploy é automático via Railway a cada push na branch `main`.

```
git push origin main  →  Railway detecta  →  Dockerfile build  →  Deploy
```

### Configuração Railway (`railway.toml`)

```toml
[build]
builder = "dockerfile"
dockerfilePath = "Dockerfile"

[deploy]
healthcheckPath = "/healthz"
healthcheckTimeout = 10
restartPolicyType = "on_failure"
restartPolicyMaxRetries = 5
```

### Dockerfile (multi-stage)

```
Stage 1 (builder):  golang:1.22-alpine → go build -ldflags="-s -w" -o /bfa
Stage 2 (runtime):  alpine:3.20 → binário ~8MB + ca-certificates + tzdata
```

---

<div align="center">

**Desenvolvido com ☕ e Go**

</div>
