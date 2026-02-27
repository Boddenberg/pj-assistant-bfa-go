# ============================================================
# Agent (Python) - Multi-stage build
# ============================================================
FROM python:3.11-slim AS builder

WORKDIR /app

COPY pyproject.toml .
RUN pip install --no-cache-dir .

COPY . .

# --- Runtime ---
FROM python:3.11-slim

WORKDIR /app

COPY --from=builder /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY --from=builder /app .

EXPOSE 8090

CMD ["uvicorn", "app.server:app", "--host", "0.0.0.0", "--port", "8090"]
