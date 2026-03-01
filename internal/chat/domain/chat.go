// Package domain — chat.go define os tipos usados pela rota GET /v1/assistant/{customerId}.
//
// Essa rota é a "porta de entrada" do chat com IA. Diferente da rota POST /v1/assistant
// que era mais complexa, essa é simples: recebe uma query string e devolve uma string.
//
// O fluxo completo:
//  1. Usuário manda query via GET body → BFA recebe
//  2. BFA usa Strategy Pattern para decidir o que fazer (context routing)
//  3. BFA enriquece o request com dados do cliente (profile, transactions)
//  4. BFA manda pro Agent Python (POST /v1/chat)
//  5. Agent responde com answer + metadata
//  6. BFA retorna SOMENTE a string answer pro chamador
package domain

// ============================================================
// Chat — Request/Response entre o chamador e o BFA
// ============================================================

// HistoryEntry representa uma troca de mensagem anterior na conversa.
type HistoryEntry struct {
	Query  string `json:"query"`
	Answer string `json:"answer"`
}

// ChatRequest é o body que o chamador envia no POST /v1/chat.
type ChatRequest struct {
	Query   string         `json:"query"`
	History []HistoryEntry `json:"history,omitempty"`
}

// ChatResponse é o que o BFA devolve pro chamador.
type ChatResponse struct {
	Answer           string   `json:"answer"`
	Context          string   `json:"context,omitempty"`
	Intent           string   `json:"intent,omitempty"`
	Confidence       float64  `json:"confidence,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`
}

// ============================================================
// Chat — Request/Response entre o BFA e o Agent Python
// ============================================================

// ChatAgentRequest é o payload que o BFA envia pro Agent Python (POST /v1/chat).
// Deve casar com o contrato do endpoint Python:
//
//	curl -X POST /v1/chat -d '{"query": "..."}'
//
// Campos adicionais (customer_id, context, journey_state) são opcionais
// e servem para o agent ter contexto da conversa/jornada.
type ChatAgentRequest struct {
	// Query é o prompt do usuário — campo obrigatório
	Query string `json:"query"`

	// CustomerID identifica o cliente PJ — usado pelo agent para personalizar a resposta
	CustomerID string `json:"customer_id,omitempty"`

	// Context indica o assunto/domínio atual da conversa.
	// Exemplos: "onboarding", "pix", "billing", "general"
	// O agent pode usar isso para ajustar o comportamento.
	Context string `json:"context,omitempty"`

	// History é o histórico da conversa para manter contexto entre turnos.
	History []HistoryEntry `json:"history,omitempty"`

	// JourneyState é o estado atual da jornada (ex: abertura de conta).
	// O agent usa isso para saber em que etapa o usuário está.
	JourneyState *JourneyState `json:"journey_state,omitempty"`
}

// ChatAgentResponse é a resposta que o Agent Python devolve.
// Baseado no contrato real do agent:
//
//	{
//	  "customer_id": "ab84533a-...",
//	  "answer": "Como já conversamos, além do CNPJ...",
//	  "context": "onboarding",
//	  "intent": "open_account",
//	  "confidence": 0.95,
//	  "suggested_actions": ["Iniciar abertura", ...],
//	  "metadata": {
//	    "reasoning": [...],
//	    "sources": ["kb: abertura_conta"],
//	    "tokens_used": 1200,
//	    "estimated_cost_usd": 0.00018
//	  },
//	  "timestamp": "2026-03-01T15:30:00.000000"
//	}
type ChatAgentResponse struct {
	CustomerID       string         `json:"customer_id"`
	Answer           string         `json:"answer"`
	Context          string         `json:"context,omitempty"`
	Intent           string         `json:"intent,omitempty"`
	Confidence       float64        `json:"confidence,omitempty"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
	Metadata         *AgentMetadata `json:"metadata,omitempty"`
	Timestamp        string         `json:"timestamp"`
}

// AgentMetadata contém informações internas do processamento do Agent.
type AgentMetadata struct {
	Reasoning  []string `json:"reasoning,omitempty"`
	Sources    []string `json:"sources,omitempty"`
	TokensUsed int      `json:"tokens_used,omitempty"`
	EstCostUSD float64  `json:"estimated_cost_usd,omitempty"`
}

// ============================================================
// Jornada (Journey) — State Machine para fluxos multi-etapa
// ============================================================

// JourneyState armazena o estado de uma jornada em andamento.
// Usado pelo Strategy Pattern para saber em que etapa o usuário está
// e quais dados já foram coletados.
//
// Para abertura de conta (onboarding), as etapas são:
//
//	Stage 1: Dados da empresa (CNPJ, razão social, nome fantasia, email)
//	Stage 2: Dados do representante (nome, CPF, telefone, data nascimento)
//	Stage 3: Senha (senha, confirmação, aceite de termos)
type JourneyState struct {
	// JourneyType identifica o tipo de jornada: "onboarding", "pix_transfer", etc.
	JourneyType string `json:"journey_type"`

	// Stage é a etapa atual (1, 2, 3...)
	Stage int `json:"stage"`

	// Status indica o estado geral: "in_progress", "completed", "cancelled", "error"
	Status string `json:"status"`

	// CollectedData armazena os dados já coletados em etapas anteriores.
	// É um mapa livre porque cada jornada tem campos diferentes.
	// Ex para onboarding stage 2: {"cnpj": "12345678000190", "razaoSocial": "Empresa X"}
	CollectedData map[string]string `json:"collected_data,omitempty"`

	// ValidationErrors lista erros de validação da etapa atual.
	// O agent pode usar isso para pedir correção ao usuário.
	ValidationErrors []string `json:"validation_errors,omitempty"`
}

// ============================================================
// Strategy Context — define qual strategy processar a mensagem
// ============================================================

// ChatContext encapsula tudo que uma Strategy precisa para processar
// uma mensagem do chat. É montado pelo ChatService antes de delegar.
type ChatContext struct {
	// CustomerID do cliente PJ
	CustomerID string

	// Query é o prompt original do usuário
	Query string

	// DetectedIntent é a intenção detectada pelo roteador.
	// Exemplos: "onboarding", "pix", "balance", "general"
	DetectedIntent string

	// History é o histórico da conversa vindo do frontend
	History []HistoryEntry

	// Journey é o estado da jornada em andamento (nil se não houver)
	Journey *JourneyState
}
