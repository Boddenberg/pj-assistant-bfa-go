// Package domain — chat.go define os tipos usados pelo módulo de chat.
//
// ============================================================
// CONTRATO v9.0.0 — BFA ↔ Agente Python ↔ Frontend
// ============================================================
//
// O fluxo completo:
//  1. Frontend envia query + history (enriquecido com step/validated)
//  2. BFA detecta intent, roteia para a strategy correta
//  3. Strategy chama o Agent Python com history enriquecido
//  4. Agent responde com step + field_value + next_step + answer
//  5. BFA valida o campo (se onboarding)
//  6. BFA enriquece o history com step + validated
//  7. BFA retorna answer + step + next_step pro frontend
package domain

// ============================================================
// Chat — Request/Response entre o Frontend e o BFA
// ============================================================

// HistoryEntry representa uma troca de mensagem anterior na conversa.
// Em v9, cada turno é enriquecido com step e validated para que
// o agente saiba exatamente onde estamos no onboarding.
type HistoryEntry struct {
	Query  string `json:"query"`
	Answer string `json:"answer"`

	// Step indica qual campo do onboarding este turno representa.
	// nil se não é onboarding (ex: welcome, conversa normal).
	Step *string `json:"step"`

	// Validated indica se o BFA validou o campo com sucesso.
	// true = aceito, false = rejeitado, nil = não aplicável (welcome, conversa normal).
	Validated *bool `json:"validated"`
}

// ChatRequest é o body que o frontend envia no POST /v1/chat.
type ChatRequest struct {
	Query   string         `json:"query"`
	History []HistoryEntry `json:"history,omitempty"`
}

// ChatResponse é o que o BFA devolve pro frontend.
type ChatResponse struct {
	Answer           string       `json:"answer"`
	Context          string       `json:"context,omitempty"`
	Intent           string       `json:"intent,omitempty"`
	Confidence       float64      `json:"confidence,omitempty"`
	Step             *string      `json:"step"`
	FieldValue       *string      `json:"field_value"`
	NextStep         *string      `json:"next_step"`
	SuggestedActions []string     `json:"suggested_actions,omitempty"`
	AccountData      *AccountData `json:"account_data,omitempty"`
}

// AccountData contém os dados da conta criada no onboarding.
// Só é preenchido quando next_step == "completed".
type AccountData struct {
	CustomerID string `json:"customerId"`
	Agencia    string `json:"agencia"`
	Conta      string `json:"conta"`
}

// ============================================================
// Chat — Request/Response entre o BFA e o Agent Python
// ============================================================

// ChatAgentRequest é o payload que o BFA envia pro Agent Python (POST /v1/chat).
//
// Em v9, o history é enriquecido com step/validated para que o agente
// saiba exatamente onde estamos no onboarding. O agente controla retries
// contando turnos com validated=false para o mesmo step.
type ChatAgentRequest struct {
	// Query é o prompt do usuário — campo obrigatório
	Query string `json:"query"`

	// CustomerID identifica o cliente PJ
	CustomerID string `json:"customer_id,omitempty"`

	// Context indica o assunto/domínio atual da conversa.
	Context string `json:"context,omitempty"`

	// History é o histórico enriquecido (com step + validated por turno).
	History []HistoryEntry `json:"history,omitempty"`

	// ValidationError é a mensagem de erro quando o BFA rejeitou o último campo.
	// Se vazio, o agente avança normalmente.
	ValidationError string `json:"validation_error,omitempty"`

	// Profile e Transactions são opcionais — usados para consultas.
	Profile      any   `json:"profile,omitempty"`
	Transactions []any `json:"transactions,omitempty"`
}

// ChatAgentResponse é a resposta que o Agent Python devolve.
//
// Em v9:
//   - step: qual campo o cliente acabou de responder (BFA valida)
//   - field_value: valor cru extraído da query
//   - next_step: próximo campo que será pedido
type ChatAgentResponse struct {
	CustomerID       string         `json:"customer_id"`
	Answer           string         `json:"answer"`
	Context          string         `json:"context,omitempty"`
	Intent           string         `json:"intent,omitempty"`
	Confidence       float64        `json:"confidence,omitempty"`
	Step             *string        `json:"step"`
	FieldValue       *string        `json:"field_value"`
	NextStep         *string        `json:"next_step"`
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
type OnboardingSession struct {
	// Started indica se o onboarding já iniciou (welcome recebido)
	Started bool

	// CollectedData guarda os campos já validados e aceitos pelo BFA.
	CollectedData map[string]string

	// EnrichedHistory é o histórico enriquecido com step/validated.
	// Mantido pelo BFA e enviado ao agente em cada turno.
	EnrichedHistory []HistoryEntry

	// LastQuery guarda a última query do cliente
	LastQuery string
}

// NextExpectedField retorna o próximo campo que ainda não foi coletado.
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
// Strategy Context
// ============================================================

// ChatContext encapsula tudo que uma Strategy precisa para processar
// uma mensagem do chat.
type ChatContext struct {
	CustomerID      string
	Query           string
	DetectedIntent  string
	History         []HistoryEntry
	ValidationError string
}

// ============================================================
// Helpers
// ============================================================

// BoolPtr retorna um ponteiro para um bool.
func BoolPtr(b bool) *bool {
	return &b
}

// StringPtr retorna um ponteiro para uma string.
func StringPtr(s string) *string {
	return &s
}
