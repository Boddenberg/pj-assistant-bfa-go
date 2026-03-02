"""Pydantic models (schemas) shared across the agent."""

from __future__ import annotations

from pydantic import BaseModel, Field


class CustomerProfile(BaseModel):
    customer_id: str
    name: str
    document: str
    segment: str
    monthly_revenue: float = 0.0
    account_age_months: int = 0
    credit_score: int = 0


class Transaction(BaseModel):
    id: str
    date: str
    amount: float
    type: str
    category: str
    description: str = ""


class AgentRequest(BaseModel):
    """Incoming request from the BFA (Go)."""

    customer_id: str
    profile: CustomerProfile
    transactions: list[Transaction] = Field(default_factory=list)
    query: str | None = None


class TokenUsage(BaseModel):
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0


class AgentResponse(BaseModel):
    """Outgoing response to the BFA (Go)."""

    answer: str
    reasoning: str
    sources: list[str] = Field(default_factory=list)
    confidence: float = 0.0
    tokens_used: TokenUsage = Field(default_factory=TokenUsage)
    tools_executed: list[str] = Field(default_factory=list)


class AgentState(BaseModel):
    """Internal state passed through the LangGraph workflow."""

    customer_id: str = ""
    profile: CustomerProfile | None = None
    transactions: list[Transaction] = Field(default_factory=list)
    query: str = ""

    # Planning
    plan: list[str] = Field(default_factory=list)

    # RAG
    retrieved_context: str = ""
    rag_sources: list[str] = Field(default_factory=list)

    # Execution
    tool_results: dict = Field(default_factory=dict)
    tools_executed: list[str] = Field(default_factory=list)

    # Response
    answer: str = ""
    reasoning: str = ""
    confidence: float = 0.0

    # Metrics
    token_usage: TokenUsage = Field(default_factory=TokenUsage)
    errors: list[str] = Field(default_factory=list)


# ============================================================
# Chat — contrato simples usado pela rota POST /v1/chat
# ============================================================


class ChatHistoryEntry(BaseModel):
    """Uma troca de mensagem anterior na conversa."""

    query: str
    answer: str


class ChatRequest(BaseModel):
    """Request do BFA (Go) via POST /v1/chat."""

    query: str
    customer_id: str = "anonymous"
    context: str = ""
    history: list[ChatHistoryEntry] = Field(default_factory=list)
    validation_error: str = ""
    profile: dict | None = None
    transactions: list[dict] = Field(default_factory=list)


class ChatMetadata(BaseModel):
    """Metadata interna do processamento (não exposta ao frontend)."""

    reasoning: list[str] = Field(default_factory=list)
    sources: list[str] = Field(default_factory=list)
    tokens_used: int = 0
    estimated_cost_usd: float = 0.0


class ChatResponse(BaseModel):
    """Response devolvida ao BFA (Go)."""

    customer_id: str = ""
    answer: str
    context: str | None = None
    intent: str | None = None
    confidence: float = 0.0
    current_field: str | None = None
    field_value: str | None = None
    suggested_actions: list[str] = Field(default_factory=list)
    metadata: ChatMetadata = Field(default_factory=ChatMetadata)
    timestamp: str = ""
