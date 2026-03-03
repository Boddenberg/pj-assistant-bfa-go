package chat

/*
 * Modelos — contratos exatos com o Agent Python e o frontend
 */

/* Frontend → BFA */

type FrontendRequest struct {
	Query string `json:"query"`
}

/* BFA → Frontend */

type FrontendResponse struct {
	Answer      string       `json:"answer"`
	Context     *string      `json:"context,omitempty"`
	Step        *string      `json:"step,omitempty"`
	NextStep    *string      `json:"next_step,omitempty"`
	AccountData *AccountData `json:"account_data,omitempty"`
}

/* BFA → Agent Python */

type AgentRequest struct {
	CustomerID       string            `json:"customer_id"`
	Query            string            `json:"query"`
	History          []ChatMessage     `json:"history"`
	ValidationError  string            `json:"validation_error"`
	CollectedData    []CollectedItem   `json:"collected_data"`
	FinancialContext *FinancialContext `json:"financial_context,omitempty"`
}

/*
 * FinancialContext — dados financeiros do cliente enviados ao Agent Python.
 * O BFA monta este bloco antes de chamar o agente, para que ele possa
 * responder perguntas sobre saldo, cartões, PIX, faturas, etc.
 *
 * Cada sub-struct é preenchida por um "context provider" independente.
 * Se o provider falhar, o campo fica nil — o agente opera com dados parciais.
 */

// FinancialContext agrupa todos os contextos financeiros do cliente.
type FinancialContext struct {
	Account    *AccountContext    `json:"account,omitempty"`
	Cards      *CardsContext      `json:"cards,omitempty"`
	Pix        *PixContext        `json:"pix,omitempty"`
	Billing    *BillingContext    `json:"billing,omitempty"`
	Profile    *ProfileContext    `json:"profile,omitempty"`
	FetchedAt  string             `json:"fetched_at"`            // RFC3339
	ContextKeys []string          `json:"context_keys"`          // quais sub-contextos foram preenchidos
}

// AccountContext — saldo, limites e dados da conta corrente.
type AccountContext struct {
	AccountID            string  `json:"account_id"`
	Branch               string  `json:"branch"`
	AccountNumber        string  `json:"account_number"`
	Balance              float64 `json:"balance"`
	AvailableBalance     float64 `json:"available_balance"`
	OverdraftLimit       float64 `json:"overdraft_limit"`
	CreditLimit          float64 `json:"credit_limit"`
	AvailableCreditLimit float64 `json:"available_credit_limit"`
	Status               string  `json:"status"`
}

// CardsContext — cartões de crédito do cliente + faturas abertas.
type CardsContext struct {
	Cards    []CardSummary    `json:"cards"`
	Invoices []InvoiceSummary `json:"invoices,omitempty"`
}

// CardSummary — resumo de um cartão de crédito.
type CardSummary struct {
	CardID         string  `json:"card_id"`
	Last4          string  `json:"last4"`
	Brand          string  `json:"brand"`
	CardType       string  `json:"card_type"`
	Status         string  `json:"status"`
	CreditLimit    float64 `json:"credit_limit"`
	AvailableLimit float64 `json:"available_limit"`
	UsedLimit      float64 `json:"used_limit"`
	DueDay         int     `json:"due_day"`
	BillingDay     int     `json:"billing_day"`
}

// InvoiceSummary — resumo de uma fatura de cartão.
type InvoiceSummary struct {
	CardID         string  `json:"card_id"`
	ReferenceMonth string  `json:"reference_month"`
	TotalAmount    float64 `json:"total_amount"`
	MinimumPayment float64 `json:"minimum_payment"`
	DueDate        string  `json:"due_date"`
	Status         string  `json:"status"`
}

// PixContext — chaves PIX e transferências recentes.
type PixContext struct {
	Keys              []PixKeySummary      `json:"keys"`
	RecentTransfers   []PixTransferSummary `json:"recent_transfers,omitempty"`
	ScheduledTransfers []ScheduledSummary  `json:"scheduled_transfers,omitempty"`
}

// PixKeySummary — resumo de uma chave PIX.
type PixKeySummary struct {
	KeyType  string `json:"key_type"`
	KeyValue string `json:"key_value"`
	Status   string `json:"status"`
}

// PixTransferSummary — resumo de uma transferência PIX.
type PixTransferSummary struct {
	TransferID      string  `json:"transfer_id"`
	Amount          float64 `json:"amount"`
	DestinationName string  `json:"destination_name"`
	Status          string  `json:"status"`
	FundedBy        string  `json:"funded_by"`
	CreatedAt       string  `json:"created_at"`
}

// ScheduledSummary — resumo de uma transferência agendada.
type ScheduledSummary struct {
	TransferID      string  `json:"transfer_id"`
	Amount          float64 `json:"amount"`
	DestinationName string  `json:"destination_name,omitempty"`
	ScheduledFor    string  `json:"scheduled_for"`
	Status          string  `json:"status"`
}

// BillingContext — boletos e compras no débito recentes.
type BillingContext struct {
	RecentBills     []BillSummary  `json:"recent_bills,omitempty"`
	RecentDebits    []DebitSummary `json:"recent_debits,omitempty"`
}

// BillSummary — resumo de um pagamento de boleto.
type BillSummary struct {
	BillID      string  `json:"bill_id"`
	Amount      float64 `json:"amount"`
	Beneficiary string  `json:"beneficiary"`
	DueDate     string  `json:"due_date"`
	Status      string  `json:"status"`
}

// DebitSummary — resumo de uma compra no débito.
type DebitSummary struct {
	Amount       float64 `json:"amount"`
	MerchantName string  `json:"merchant_name"`
	Category     string  `json:"category"`
	Date         string  `json:"date"`
	Status       string  `json:"status"`
}

// ProfileContext — dados cadastrais do cliente PJ.
type ProfileContext struct {
	CustomerID  string `json:"customer_id"`
	CompanyName string `json:"company_name"`
	Document    string `json:"document"` // CNPJ
	Segment     string `json:"segment"`
	Email       string `json:"email,omitempty"`
}

// CollectedItem representa um dado já validado pelo BFA.
// Genérico — serve para qualquer jornada (onboarding, pix, etc).
type CollectedItem struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Validated bool   `json:"validated"`
}

/* Agent Python → BFA */

type AgentResponse struct {
	CustomerID       string         `json:"customer_id"`
	Answer           string         `json:"answer"`
	RagContexts      []string       `json:"rag_contexts"`
	Context          *string        `json:"context"`
	Intent           *string        `json:"intent"`
	Confidence       float64        `json:"confidence"`
	Step             *string        `json:"step"`
	FieldValue       *string        `json:"field_value"`
	NextStep         *string        `json:"next_step"`
	SuggestedActions []string       `json:"suggested_actions"`
	Metadata         map[string]any `json:"metadata"`
	Timestamp        string         `json:"timestamp"`
}

/* History entry */

type ChatMessage struct {
	Query     string  `json:"query"`
	Answer    string  `json:"answer"`
	Step      *string `json:"step"`
	Validated *bool   `json:"validated"`
}

/* Session (em memória, por customer_id) */

type Session struct {
	CustomerID     string
	History        []ChatMessage
	OnboardingData map[string]string
	LastStep       string // último step em que estávamos (para controle de retries)
	Retries        int    // quantas tentativas inválidas consecutivas no step atual
}

const MaxRetries = 3

// RequiredOnboardingFields são os campos obrigatórios para abrir uma conta PJ.
// Usados apenas para verificar se todos foram preenchidos antes de finalizar.
// A ORDEM é decidida pelo agente, NÃO pelo BFA.
var RequiredOnboardingFields = []string{
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

// MissingFields retorna os campos obrigatórios que ainda não foram validados na sessão.
func (s *Session) MissingFields() []string {
	var missing []string
	for _, field := range RequiredOnboardingFields {
		if _, ok := s.OnboardingData[field]; !ok {
			missing = append(missing, field)
		}
	}
	return missing
}

// CollectedData converte os dados já validados da sessão em lista genérica de chave/valor.
// Não expõe password/passwordConfirmation ao agente.
func (s *Session) CollectedData() []CollectedItem {
	items := make([]CollectedItem, 0)
	for key, value := range s.OnboardingData {
		// Não enviar senhas ao agente
		if key == "password" || key == "passwordConfirmation" {
			continue
		}
		items = append(items, CollectedItem{
			Key:       key,
			Value:     value,
			Validated: true,
		})
	}
	return items
}

// helper para ponteiro de string
func strPtr(s string) *string { return &s }

// helper para ponteiro de bool
func boolPtr(b bool) *bool { return &b }

/* BFA → Agent Python: LLM-as-Judge */

// EvaluateRequest é enviado ao agente para avaliação via LLM-as-Judge.
// Contém a transcrição completa da conversa do cliente.
type EvaluateRequest struct {
	CustomerID   string            `json:"customer_id"`
	Conversation []TranscriptEntry `json:"conversation"` // lista completa de turnos
}

// TranscriptEntry é um turno individual da conversa (query + answer + metadados).
type TranscriptEntry struct {
	Query               string   `json:"query"`
	Answer              string   `json:"answer"`
	Contexts            []string `json:"contexts"`
	Step                string   `json:"step,omitempty"`
	Intent              string   `json:"intent,omitempty"`
	Confidence          float64  `json:"confidence,omitempty"`
	LatencyMs           int64    `json:"latency_ms,omitempty"`
	CreatedAt           string   `json:"created_at"`
	FinancialContextKeys []string `json:"financial_context_keys,omitempty"` // quais contextos foram enviados
}

/* Agent Python → BFA: resposta do LLM-as-Judge */

// EvaluateResponse é a resposta do agente após avaliar a conversa via LLM-as-Judge.
type EvaluateResponse struct {
	CustomerID   string                `json:"customer_id"`
	OverallScore float64               `json:"overall_score"`
	Verdict      string                `json:"verdict"` // "pass" | "fail" | "warning"
	Criteria     []EvaluationCriterion `json:"criteria"`
	Summary      string                `json:"summary"`
	Improvements []string              `json:"improvements"`
	NumTurns     int                   `json:"num_turns"`
	Metadata     EvaluationMetadata    `json:"metadata"`
	Timestamp    string                `json:"timestamp"`
}

// EvaluationCriterion é um critério individual avaliado pelo juiz.
type EvaluationCriterion struct {
	Criterion string  `json:"criterion"`
	Score     float64 `json:"score"`
	MaxScore  float64 `json:"max_score"`
	Reasoning string  `json:"reasoning"`
}

// EvaluationMetadata contém informações sobre o processo de avaliação.
type EvaluationMetadata struct {
	JudgeModel         string  `json:"judge_model"`
	JudgePromptVersion string  `json:"judge_prompt_version"`
	TokensUsed         int     `json:"tokens_used"`
	EstimatedCostUSD   float64 `json:"estimated_cost_usd"`
	EvalDurationMs     float64 `json:"evaluation_duration_ms"`
}
