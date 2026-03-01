package domain

import "time"

// ============================================================
// Agente IA
// ============================================================

// AgentRequest é o payload enviado para o serviço do Agente IA.
type AgentRequest struct {
	CustomerID   string              `json:"customer_id"`
	Profile      *CustomerProfile    `json:"profile"`
	Transactions []Transaction       `json:"transactions"`
	Summary      *TransactionSummary `json:"summary,omitempty"`
	Query        string              `json:"query,omitempty"`
}

// AgentResponse contém a resposta estruturada do Agente IA.
type AgentResponse struct {
	Answer        string     `json:"answer"`
	Reasoning     string     `json:"reasoning"`
	Sources       []string   `json:"sources,omitempty"`
	Confidence    float64    `json:"confidence"`
	TokensUsed    TokenUsage `json:"tokens_used"`
	ToolsExecuted []string   `json:"tools_executed,omitempty"`
}

// TokenUsage rastreia o consumo de tokens do LLM para monitoramento de custos.
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUsd float64 `json:"estimatedCostUsd,omitempty"`
}

// ============================================================
// API do Assistente — Request/Response (segue o contrato do frontend)
// ============================================================

// AssistantRequest é o body do POST /v1/assistant/{customerId}.
type AssistantRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversationId,omitempty"`
}

// AssistantMessage representa uma mensagem individual do chat.
type AssistantMessage struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"` // user, assistant, system (papel na conversa)
	Content   string           `json:"content"`
	Timestamp string           `json:"timestamp"`
	Metadata  *MessageMetadata `json:"metadata,omitempty"`
}

// MessageMetadata enriquece a mensagem com informações de tools/RAG/tokens.
type MessageMetadata struct {
	ToolsUsed  []string    `json:"toolsUsed,omitempty"`
	RAGSources []RAGSource `json:"ragSources,omitempty"`
	TokenUsage *TokenUsage `json:"tokenUsage,omitempty"`
	LatencyMs  int64       `json:"latencyMs,omitempty"`
	Reasoning  string      `json:"reasoning,omitempty"`
}

// RAGSource representa uma fonte de documento usada pelo pipeline RAG.
type RAGSource struct {
	DocumentID     string  `json:"documentId"`
	Title          string  `json:"title"`
	RelevanceScore float64 `json:"relevanceScore"`
	Snippet        string  `json:"snippet"`
}

// AssistantResponse é a resposta final do endpoint do assistente BFA.
type AssistantResponse struct {
	ConversationID string            `json:"conversationId"`
	Message        *AssistantMessage `json:"message"`
	Profile        *CustomerProfile  `json:"profile,omitempty"`
	Transactions   []Transaction     `json:"transactions,omitempty"`
}

// InternalAssistantResult é o resultado no nível de serviço antes de mapear para o formato da API.
type InternalAssistantResult struct {
	CustomerID     string
	Profile        *CustomerProfile
	Recommendation *AgentResponse
	ProcessedAt    time.Time
}
