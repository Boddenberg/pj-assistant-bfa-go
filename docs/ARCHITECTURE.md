# Arquitetura — PJ Assistant BFA + AI Agent

## Diagrama Geral

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Cliente (App/Web)                           │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │ GET /v1/assistant/{customerId}
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        BFA (Go) :8080                               │
│                                                                     │
│  ┌──────────┐  ┌────────────┐  ┌───────────┐  ┌─────────────────┐  │
│  │  Router   │  │ Middleware │  │  Handler  │  │    Service      │  │
│  │  (chi)    │──│  Logging   │──│  JSON     │──│  Orchestrator   │  │
│  │          │  │  Tracing   │  │  Errors   │  │                 │  │
│  └──────────┘  └────────────┘  └───────────┘  └────────┬────────┘  │
│                                                         │           │
│  ┌──────────────────────────────────────────────────────┤           │
│  │            Concurrent Calls (errgroup)               │           │
│  │                                                      │           │
│  │  ┌─────────────┐  ┌──────────────────┐              │           │
│  │  │ Profile     │  │ Transactions     │              │           │
│  │  │ Client      │  │ Client           │              │           │
│  │  │ +retry      │  │ +retry           │              │           │
│  │  │ +circuit    │  │ +circuit         │              │           │
│  │  │  breaker    │  │  breaker         │              │           │
│  │  └──────┬──────┘  └────────┬─────────┘              │           │
│  │         │                   │                        │           │
│  └─────────┼───────────────────┼────────────────────────┘           │
│            │                   │                                    │
│  ┌─────────┴────┐  ┌─────────┴────┐   ┌─────────────────────┐     │
│  │ Cache (TTL)  │  │              │   │ Agent Client        │     │
│  │ In-Memory    │  │              │   │ +retry +circuit     │     │
│  └──────────────┘  │              │   └──────────┬──────────┘     │
│                     │              │              │                 │
│  ┌──────────────────┴──────────────┴──────────────┼───────────┐    │
│  │  Observability: Prometheus metrics | OTel traces | Zap logs│    │
│  └────────────────────────────────────────────────────────────┘    │
└───────────────────────────────────────────┬─────────────────────────┘
                                            │ POST /v1/agent/invoke
                                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    AI Agent (Python/FastAPI) :8090                   │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                   Security Layer                              │   │
│  │  Input Sanitization | Injection Detection | PII Redaction     │   │
│  │  Rate Limiting | Cost Control                                 │   │
│  └──────────────────────────┬───────────────────────────────────┘   │
│                              │                                      │
│  ┌──────────────────────────▼───────────────────────────────────┐   │
│  │                   LangGraph Workflow                          │   │
│  │                                                               │   │
│  │   [START]                                                     │   │
│  │      │                                                        │   │
│  │      ▼                                                        │   │
│  │   ┌──────────┐                                                │   │
│  │   │ Planner  │ ── Decide which steps are needed               │   │
│  │   └────┬─────┘                                                │   │
│  │        │                                                      │   │
│  │        ▼ (conditional)                                        │   │
│  │   ┌──────────┐                                                │   │
│  │   │Retriever │ ── RAG: semantic search in knowledge base      │   │
│  │   └────┬─────┘                                                │   │
│  │        │                                                      │   │
│  │        ▼ (conditional)                                        │   │
│  │   ┌──────────┐                                                │   │
│  │   │ Analyzer │ ── Financial data analysis (deterministic)     │   │
│  │   └────┬─────┘                                                │   │
│  │        │                                                      │   │
│  │        ▼                                                      │   │
│  │   ┌──────────────┐                                            │   │
│  │   │ Synthesizer  │ ── LLM call to generate recommendation    │   │
│  │   └──────┬───────┘                                            │   │
│  │          │                                                    │   │
│  │       [END]                                                   │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  RAG Pipeline                                                 │   │
│  │  Knowledge Base (txt) → Chunking → Embeddings → ChromaDB     │   │
│  │  Query → Embedding → Similarity Search → Filter → Context    │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  Observability: Prometheus | Structured Logging               │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
          │                                     │
          ▼                                     ▼
┌──────────────────┐                 ┌────────────────────┐
│  Jaeger :16686   │                 │ Prometheus :9090   │
│  Distributed     │                 │ Metrics            │
│  Tracing         │                 │ Dashboards         │
└──────────────────┘                 └────────────────────┘
```

## Fluxo de Dados

```
1. Cliente faz GET /v1/assistant/{customerId}
2. BFA (Go) recebe a request
3. Concorrentemente (errgroup):
   a. Busca Profile API (com cache + retry + circuit breaker)
   b. Busca Transactions API (com retry + circuit breaker)
4. Monta payload e chama POST /v1/agent/invoke
5. Agent (Python):
   a. Valida segurança (rate limit, injection, sanitização)
   b. Planner decide os passos necessários
   c. Retriever faz busca semântica na base de conhecimento
   d. Analyzer processa dados financeiros
   e. Synthesizer chama LLM para gerar recomendação
   f. Retorna resposta estruturada com métricas de tokens
6. BFA recebe resposta, registra métricas e retorna ao cliente
```

## Decisões Arquiteturais

### Separação BFA × Agent
- **BFA (Go)**: Orquestração, resiliência, performance — Go é ideal para I/O concorrente
- **Agent (Python)**: Ecossistema de IA — LangChain/LangGraph, modelos de embedding, ChromaDB

### Resiliência (BFA)
- **Retry com backoff exponencial + jitter**: evita thundering herd
- **Circuit Breaker (gobreaker)**: protege contra cascading failures
- **Bulkhead**: limita concorrência por recurso
- **Timeout via context.Context**: propagado em toda a cadeia

### RAG
- **Chunking**: RecursiveCharacterTextSplitter (500 chars, 100 overlap)
- **Embeddings**: all-MiniLM-L6-v2 (leve, roda em CPU)
- **Vector Store**: ChromaDB (local, sem infra adicional)
- **Filtragem**: score threshold para evitar contexto irrelevante

### Segurança
- Sanitização de input no boundary (agent)
- Detecção de prompt injection via regex patterns
- Redação de PII na resposta
- Rate limiting por customer
- Controle de custo diário por customer

## Estratégia de Deploy (AWS)

```
┌─────────────────────────────────────────────────┐
│                    AWS                           │
│                                                  │
│  ┌─────────────┐     ┌─────────────────────┐    │
│  │   ALB        │────▶│  ECS Fargate        │    │
│  │   /v1/*      │     │  ┌──────┐ ┌──────┐  │    │
│  └─────────────┘     │  │ BFA  │ │Agent │  │    │
│                       │  │ (Go) │ │ (Py) │  │    │
│                       │  └──┬───┘ └──┬───┘  │    │
│                       └─────┼────────┼──────┘    │
│                             │        │            │
│  ┌──────────────────────────┼────────┼────────┐  │
│  │  VPC Private Subnets     │        │        │  │
│  │                          ▼        ▼        │  │
│  │  ┌──────────┐  ┌──────────┐  ┌─────────┐  │  │
│  │  │ ElastiC. │  │ S3       │  │ CloudW. │  │  │
│  │  │ (cache)  │  │ (KB docs)│  │ (logs)  │  │  │
│  │  └──────────┘  └──────────┘  └─────────┘  │  │
│  └────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────┘
```
