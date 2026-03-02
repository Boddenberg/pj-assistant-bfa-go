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
	Answer   string  `json:"answer"`
	Context  *string `json:"context,omitempty"`
	Step     *string `json:"step,omitempty"`
	NextStep *string `json:"next_step,omitempty"`
}

// --- BFA → Agent Python ---

type AgentRequest struct {
	CustomerID      string        `json:"customer_id"`
	Query           string        `json:"query"`
	History         []ChatMessage `json:"history"`
	ValidationError string        `json:"validation_error"`
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
}

// helper para ponteiro de string
func strPtr(s string) *string { return &s }

// helper para ponteiro de bool
func boolPtr(b bool) *bool { return &b }
