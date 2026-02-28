package domain

import "time"

// ============================================================
// AI Agent
// ============================================================

// AgentRequest is sent to the AI Agent service.
type AgentRequest struct {
	CustomerID   string              `json:"customer_id"`
	Profile      *CustomerProfile    `json:"profile"`
	Transactions []Transaction       `json:"transactions"`
	Summary      *TransactionSummary `json:"summary,omitempty"`
	Query        string              `json:"query,omitempty"`
}

// AgentResponse holds the AI Agent's structured response.
type AgentResponse struct {
	Answer        string     `json:"answer"`
	Reasoning     string     `json:"reasoning"`
	Sources       []string   `json:"sources,omitempty"`
	Confidence    float64    `json:"confidence"`
	TokensUsed    TokenUsage `json:"tokens_used"`
	ToolsExecuted []string   `json:"tools_executed,omitempty"`
}

// TokenUsage tracks LLM token consumption for cost monitoring.
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUsd float64 `json:"estimatedCostUsd,omitempty"`
}

// ============================================================
// Assistant API Request/Response (matches frontend spec)
// ============================================================

// AssistantRequest is the POST body for /v1/assistant/{customerId}.
type AssistantRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversationId,omitempty"`
}

// AssistantMessage represents a single chat message.
type AssistantMessage struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"` // user, assistant, system
	Content   string           `json:"content"`
	Timestamp string           `json:"timestamp"`
	Metadata  *MessageMetadata `json:"metadata,omitempty"`
}

// MessageMetadata enriches a message with tool/RAG/token info.
type MessageMetadata struct {
	ToolsUsed  []string    `json:"toolsUsed,omitempty"`
	RAGSources []RAGSource `json:"ragSources,omitempty"`
	TokenUsage *TokenUsage `json:"tokenUsage,omitempty"`
	LatencyMs  int64       `json:"latencyMs,omitempty"`
	Reasoning  string      `json:"reasoning,omitempty"`
}

// RAGSource represents a document source used by the RAG pipeline.
type RAGSource struct {
	DocumentID     string  `json:"documentId"`
	Title          string  `json:"title"`
	RelevanceScore float64 `json:"relevanceScore"`
	Snippet        string  `json:"snippet"`
}

// AssistantResponse is the final response from the BFA assistant endpoint.
type AssistantResponse struct {
	ConversationID string            `json:"conversationId"`
	Message        *AssistantMessage `json:"message"`
	Profile        *CustomerProfile  `json:"profile,omitempty"`
	Transactions   []Transaction     `json:"transactions,omitempty"`
}

// InternalAssistantResult is the service-level result before mapping to API shape.
type InternalAssistantResult struct {
	CustomerID     string
	Profile        *CustomerProfile
	Recommendation *AgentResponse
	ProcessedAt    time.Time
}
