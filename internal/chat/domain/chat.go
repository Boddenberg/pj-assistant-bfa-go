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
	Answer           string       `json:"answer"`
	Context          string       `json:"context,omitempty"`
	Intent           string       `json:"intent,omitempty"`
	Confidence       float64      `json:"confidence,omitempty"`
	CurrentField     *string      `json:"current_field"`
	FieldValue       *string      `json:"field_value"`
	SuggestedActions []string     `json:"suggested_actions,omitempty"`
	AccountData      *AccountData `json:"account_data,omitempty"`
}

// AccountData contém os dados da conta criada no onboarding.
// Só é preenchido quando current_field == "completed".
// Corresponde ao mesmo contrato do POST /v1/auth/register (RegisterResponse).
type AccountData struct {
	CustomerID string `json:"customerId"`
	Agencia    string `json:"agencia"`
	Conta      string `json:"conta"`
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

	// ValidationError é a mensagem de erro quando o BFA rejeitou o último campo.
	// Se vazio, o agente avança normalmente.
	ValidationError string `json:"validation_error,omitempty"`

	// ExpectedField indica qual campo o agente deve pedir/receber agora.
	// Preenchido pelo BFA com base na sessão de onboarding.
	// Evita que o LLM precise "adivinhar" em qual etapa está.
	ExpectedField string `json:"expected_field,omitempty"`

	// CollectedFields lista os campos já aceitos pelo BFA.
	// O agente usa para saber o que já foi coletado.
	CollectedFields []string `json:"collected_fields,omitempty"`

	// Profile e Transactions são opcionais — usados pelo /v1/agent/invoke.
	Profile      any   `json:"profile,omitempty"`
	Transactions []any `json:"transactions,omitempty"`
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
	CurrentField     *string        `json:"current_field"`
	FieldValue       *string        `json:"field_value"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
	Metadata         *AgentMetadata `json:"metadata,omitempty"`
	Timestamp        string         `json:"timestamp"`
}

// AgentMetadata contém informações internas do processamento do Agent.
type AgentMetadata struct {
	Reasoning  []map[string]interface{} `json:"reasoning,omitempty"`
	Sources    []string                 `json:"sources,omitempty"`
	TokensUsed int                      `json:"tokens_used,omitempty"`
	EstCostUSD float64                  `json:"estimated_cost_usd,omitempty"`
}

// ============================================================
// Onboarding Session — dados coletados campo a campo
// ============================================================

// OnboardingFields é a lista ordenada de campos do onboarding.
// O agente pede um por vez, sempre nesta ordem.
var OnboardingFields = []string{
	"cnpj",
	"razaoSocial",
	"nomeFantasia",
	"email",
	"representanteName",
	"representanteCpf",
	"representantePhone",
	"representanteBirthDate",
	"password",
	"passwordConfirmation",
}

// OnboardingSession armazena o estado do onboarding em andamento.
// Mantida em memória (por customerID) pelo OnboardingStrategy.
type OnboardingSession struct {
	// Started indica se o onboarding já iniciou (welcome recebido)
	Started bool

	// CollectedData guarda os campos já validados e aceitos pelo BFA.
	CollectedData map[string]string

	// LastQuery guarda a última query do cliente (para reenviar ao agent com validation_error)
	LastQuery string
}

// NextExpectedField retorna o próximo campo que ainda não foi coletado.
// Segue a ordem fixa de OnboardingFields.
// Se todos foram coletados, retorna "completed".
func (s *OnboardingSession) NextExpectedField() string {
	for _, field := range OnboardingFields {
		if _, ok := s.CollectedData[field]; !ok {
			return field
		}
	}
	return "completed"
}

// CollectedFieldNames retorna os nomes dos campos já coletados.
func (s *OnboardingSession) CollectedFieldNames() []string {
	var names []string
	for _, field := range OnboardingFields {
		if _, ok := s.CollectedData[field]; ok {
			names = append(names, field)
		}
	}
	return names
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

	// ValidationError é preenchido quando o BFA rejeita um campo.
	// O agente recebe isso e pede o campo novamente com a mensagem de erro.
	ValidationError string
}
