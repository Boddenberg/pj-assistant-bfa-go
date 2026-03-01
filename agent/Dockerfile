# ============================================================
# Agent (Python) - Multi-stage build — Railway compatible
# ============================================================
# Stage 1: builder — compila dependências C (gcc/g++ necessários)
FROM python:3.11-slim AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY pyproject.toml .

# PyTorch CPU-only (muito menor que a versão completa com CUDA)
RUN pip install --no-cache-dir \
    torch --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir .

# Limpeza segura do PyTorch — remove apenas artefatos de build/test,
# NÃO remove módulos internos (_functorch, _dynamo etc.) que o runtime precisa.
RUN find /usr/local/lib/python3.11/site-packages/torch -type d -name "test" -exec rm -rf {} + 2>/dev/null; \
    find /usr/local/lib/python3.11/site-packages/torch -type d -name "tests" -exec rm -rf {} + 2>/dev/null; \
    rm -rf /usr/local/lib/python3.11/site-packages/torch/lib/*.a 2>/dev/null; \
    rm -rf /usr/local/lib/python3.11/site-packages/torch/include 2>/dev/null; \
    rm -rf /usr/local/lib/python3.11/site-packages/torch/share 2>/dev/null; \
    find /usr/local/lib/python3.11/site-packages -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null; \
    true

# --- Stage 2: runtime (sem compiladores) ---
FROM python:3.11-slim

ARG APP_USER=appuser

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash ${APP_USER}

WORKDIR /app

# Copia pacotes Python do builder
COPY --from=builder /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin

# Pre-download do modelo de embeddings (evita download no runtime)
ENV HF_HOME=/home/${APP_USER}/.cache/huggingface
RUN python -c "from sentence_transformers import SentenceTransformer; SentenceTransformer('all-MiniLM-L6-v2')"

# Copia código do projeto
COPY . .

RUN chown -R ${APP_USER}:${APP_USER} /app
USER ${APP_USER}

EXPOSE 8090

CMD ["uvicorn", "app.server:app", "--host", "0.0.0.0", "--port", "8090"]
