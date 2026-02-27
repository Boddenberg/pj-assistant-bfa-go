# PJ Assistant — BFA (Go) + AI Agent (Python/LangGraph)

> Assistente inteligente para clientes PJ do Itaú, combinando um Backend For App (BFA) em Go com um Agente de IA Generativa baseado em LangGraph.

## 📁 Estrutura do Projeto

```
pj-assistant-bfa-go/
│
├── cmd/bfa/                        # Entrypoint do BFA (Go)
│   └── main.go
│
├── internal/                       # Código interno do BFA
│   ├── config/                     # Configuração (env vars)
│   ├── domain/                     # Modelos de domínio e erros
│   ├── handler/                    # HTTP handlers e router (chi)
│   ├── port/                       # Interfaces (ports) — hexagonal
│   ├── service/                    # Lógica de orquestração
│   └── infra/                      # Implementações de infraestrutura
│       ├── cache/                  # Cache in-memory com TTL
│       ├── client/                 # Clientes HTTP (Profile, Transactions, Agent)
│       ├── observability/          # Métricas (Prometheus), Tracing (OTel), Logging (Zap)
│       └── resilience/             # Retry, Circuit Breaker, Bulkhead
│
├── agent/                          # Agente de IA (Python)
│   ├── app/
│   │   ├── server.py               # FastAPI server
│   │   ├── config.py               # Configuração
│   │   ├── models.py               # Pydantic schemas
│   │   ├── graph.py                # LangGraph workflow
│   │   ├── security.py             # Segurança e governança
│   │   ├── observability.py        # Métricas Prometheus
│   │   ├── nodes/                  # Nós do grafo do agente
│   │   │   ├── planner.py          # Planner — decide os passos
│   │   │   ├── retriever.py        # Retriever — busca RAG
│   │   │   ├── analyzer.py         # Analyzer — análise financeira
│   │   │   └── synthesizer.py      # Synthesizer — gera recomendação via LLM
│   │   └── rag/
│   │       └── retriever.py        # Pipeline RAG (chunking, embeddings, busca)
│   ├── data/knowledge_base/        # Base de conhecimento (textos fictícios)
│   ├── tests/                      # Testes do agente
│   └── pyproject.toml
│
├── deploy/                         # Configurações de deploy
│   └── prometheus.yml
├── docs/
│   └── ARCHITECTURE.md             # Documentação arquitetural
│
├── docker-compose.yml              # Stack completa
├── Dockerfile                      # BFA (Go)
├── Makefile                        # Comandos úteis
└── README.md
```

---

## 🚀 Como Rodar Localmente

### Pré-requisitos
- Go 1.22+
- Python 3.11+
- Docker & Docker Compose (opcional)
- Uma API key da OpenAI (para o agente)

### Opção 1: Docker Compose (recomendado)

```bash
# Configure a API key
export OPENAI_API_KEY=sk-your-key-here

# Suba toda a stack
docker compose up --build

# Endpoints disponíveis:
# BFA:         http://localhost:8080/v1/assistant/{customerId}
# Agent:       http://localhost:8090/v1/agent/invoke
# Jaeger UI:   http://localhost:16686
# Prometheus:  http://localhost:9090
```

### Opção 2: Rodar separadamente

```bash
# Terminal 1 — BFA (Go)
export AGENT_API_URL=http://localhost:8090
go run ./cmd/bfa

# Terminal 2 — Agent (Python)
cd agent
cp .env.example .env  # Configure sua OPENAI_API_KEY
pip install -e ".[dev]"
uvicorn app.server:app --reload --port 8090
```

---

## 🧪 Como Rodar Testes

```bash
# Testes Go (unitários + race detection)
make test

# Testes Python (unitários + cobertura)
make agent-test

# Todos
make test-all
```

---

## 🏗 Decisões Arquiteturais

### 1. Separação BFA (Go) × Agent (Python)
**Decisão**: Dois serviços independentes, comunicando via HTTP/JSON.

**Justificativa**:
- Go é ideal para o BFA: I/O concorrente, low-latency, forte tipagem
- Python é o ecossistema dominante em IA: LangChain, LangGraph, modelos de embedding
- Separação permite escalar independentemente e equipes distintas operarem cada parte

**Trade-off**: Overhead de rede entre serviços. Em produção, poderia usar gRPC para menor latência.

### 2. Arquitetura Hexagonal (BFA)
**Decisão**: Domain, Ports, Service, Infra — separação clara de responsabilidades.

**Justificativa**:
- Testabilidade: mocks nas interfaces (ports)
- Flexibilidade: trocar implementações sem alterar domínio
- Clareza: cada pacote tem uma responsabilidade

### 3. LangGraph para o Agente
**Decisão**: Workflow estruturado como grafo com nós independentes.

**Justificativa**:
- Grafo explícito e auditável (vs. chains implícitas)
- Edges condicionais permitem pular etapas desnecessárias
- Estado tipado e rastreável entre nós
- Facilidade para adicionar novos nós (ex: multiagente)

### 4. RAG com ChromaDB + sentence-transformers
**Decisão**: Embeddings locais com modelo leve, vector store sem infra externa.

**Justificativa**:
- `all-MiniLM-L6-v2`: roda em CPU, suficiente para demonstração
- ChromaDB: zero infra, persistente em disco
- Chunking de 500 chars com 100 overlap: granularidade adequada para busca precisa

**Trade-off**: Em produção, usaria OpenSearch/Pinecone + reranking com cross-encoder.

### 5. Resiliência no BFA
- **Retry exponencial + jitter**: evita thundering herd
- **Circuit Breaker**: protege contra falhas em cascata
- **Bulkhead**: limita concorrência, evita resource starvation
- **Context com timeout**: propagado em toda a cadeia de chamadas

### 6. Segurança do Agente
- Sanitização no boundary (input cleaning)
- Detecção de prompt injection por patterns
- Redação de PII na saída
- Rate limiting por customer
- Controle de custo diário por customer

---

## ⚖️ Trade-offs Assumidos

| Decisão | Trade-off |
|---------|-----------|
| Cache in-memory (Go) | Simples, mas não compartilhado entre instâncias. Em prod: Redis/ElastiCache |
| ChromaDB local | Zero infra, mas não escala horizontalmente. Em prod: OpenSearch/Pinecone |
| Embeddings em CPU | Lento para grandes volumes. Em prod: GPU ou API de embeddings |
| HTTP entre BFA↔Agent | Overhead vs simplicidade. Em prod: gRPC com streaming |
| LLM via OpenAI API | Dependência externa. Em prod: avaliaria modelos on-premise ou Bedrock |
| Prompt injection por regex | Cobertura limitada. Em prod: combinaria com classificador ML |
| Métricas Prometheus pull | Requer scraping. Em prod: push via OTLP para observabilidade unificada |

---

## 🔄 O Que Faria Diferente em Produção Real

1. **Cache distribuído**: Redis/ElastiCache ao invés de in-memory
2. **Vector Store gerenciado**: Amazon OpenSearch com plugin k-NN ou Pinecone
3. **Reranking**: Cross-encoder para reordenar resultados do RAG
4. **gRPC**: Comunicação BFA↔Agent com Protocol Buffers
5. **Avaliação de qualidade**: LLM-as-judge para scoring automático de respostas
6. **MLOps pipeline**: Versionamento de prompts, A/B testing de modelos
7. **Event-driven**: SQS/SNS para desacoplar chamadas ao agente quando assíncrono
8. **Guardrails LLM**: NeMo Guardrails ou similar para governança de output
9. **Secrets Manager**: AWS Secrets Manager para API keys
10. **Observabilidade**: LangFuse/LangSmith para tracing específico de LLM

---

## 📊 Métricas e Qualidade

### Métricas Implementadas
- `bfa_request_duration_seconds` — Latência do BFA por operação
- `bfa_external_errors_total` — Erros de serviços externos
- `bfa_cache_hits_total` / `bfa_cache_misses_total` — Cache hit ratio
- `bfa_llm_tokens_total` — Tokens consumidos (prompt/completion)
- `agent_request_duration_seconds` — Latência do agente por step
- `agent_request_cost_usd` — Custo estimado por request
- `agent_errors_total` — Erros por tipo
- `agent_response_confidence` — Confiança da resposta
- `agent_fallback_total` — Taxa de fallback

### Como Avaliar Qualidade
- **Qualidade de resposta**: LLM-as-judge com rubrics (relevância, completude, tom)
- **Precisão do RAG**: Recall@K e precision medidos contra golden set
- **Alucinações**: Verificação de groundedness — resposta baseada nos docs recuperados
- **Drift de modelo**: Monitorar distribuição de confiança e tokens ao longo do tempo

---

## 🗺 Estratégia de Evolução Futura

1. **Multiagente**: Agentes especializados (crédito, investimento, risco) orquestrados por um meta-agente
2. **Streaming**: SSE/WebSocket para respostas em tempo real
3. **Memory**: Memória de longo prazo por customer (histórico de interações)
4. **Feedback loop**: Captura de feedback do usuário para fine-tuning
5. **Cache vetorial**: Cache semântico de respostas similares para reduzir custo de LLM
6. **Avaliação contínua**: Pipeline de avaliação automática em CI/CD

---

## 📜 Licença

Projeto desenvolvido para o case técnico — uso interno.
