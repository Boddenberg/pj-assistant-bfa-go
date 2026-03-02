package chatv2

// ============================================================
// Modelos — contratos exatos com o Agent Python e o frontend
// ============================================================

// --- Frontend → BFA ---

type FrontendRequest struct {
	Query string `json:"query"`
}

// --- BFA → Frontend ---

type FrontendResponse struct {
	Answer      string       `json:"answer"`
	Context     *string      `json:"context,omitempty"`
	Step        *string      `json:"step,omitempty"`
	NextStep    *string      `json:"next_step,omitempty"`
	AccountData *AccountData `json:"account_data,omitempty"`
}

// --- BFA → Agent Python ---

type AgentRequest struct {
	CustomerID      string          `json:"customer_id"`
	Query           string          `json:"query"`
	History         []ChatMessage   `json:"history"`
	ValidationError string          `json:"validation_error"`
	CollectedData   []CollectedItem `json:"collected_data"`
}

// CollectedItem representa um dado já validado pelo BFA.
// Genérico — serve para qualquer jornada (onboarding, pix, etc).
type CollectedItem struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Validated bool   `json:"validated"`
}

// --- Agent Python → BFA ---

type AgentResponse struct {
	CustomerID       string         `json:"customer_id"`
	Answer           string         `json:"answer"`
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

// --- History entry ---

type ChatMessage struct {
	Query     string  `json:"query"`
	Answer    string  `json:"answer"`
	Step      *string `json:"step"`
	Validated *bool   `json:"validated"`
}

// --- Session (em memória, por customer_id) ---

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
