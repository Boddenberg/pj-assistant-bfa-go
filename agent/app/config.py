"""Application configuration loaded from environment variables."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """All settings are loaded from env vars with defaults for local development."""

    # LLM
    openai_api_key: str = "sk-placeholder"
    openai_model: str = "gpt-4o-mini"

    # Embeddings
    embedding_model: str = "all-MiniLM-L6-v2"

    # Vector Store
    chroma_persist_dir: str = "./data/chroma"
    knowledge_base_dir: str = "./data/knowledge_base"

    # Supabase
    supabase_url: str = ""
    supabase_anon_key: str = ""
    supabase_service_role_key: str = ""
    use_supabase: bool = True  # Use Supabase pgvector instead of ChromaDB

    # Observability
    otel_exporter_otlp_endpoint: str = "http://localhost:4317"
    log_level: str = "info"

    # Cost control
    max_tokens_per_request: int = 4096
    cost_per_1k_prompt_tokens: float = 0.00015
    cost_per_1k_completion_tokens: float = 0.0006

    # Security
    max_input_length: int = 5000
    rate_limit_per_user: int = 100  # requests per hour

    model_config = {"env_file": ".env", "extra": "ignore"}


settings = Settings()
