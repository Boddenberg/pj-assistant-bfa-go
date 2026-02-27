"""
Observability module for the AI Agent.

Provides:
- Prometheus metrics (latency, tokens, costs, errors)
- Structured logging via structlog
- OpenTelemetry tracing setup
"""

from __future__ import annotations

import time
from contextlib import contextmanager

from prometheus_client import Counter, Histogram, Gauge


# --- Prometheus Metrics ---

AGENT_LATENCY = Histogram(
    "agent_request_duration_seconds",
    "Duration of agent requests",
    ["step"],  # planner, retriever, analyzer, synthesizer, total
    buckets=[0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0],
)

TOKENS_USED = Counter(
    "agent_llm_tokens_total",
    "Total LLM tokens consumed",
    ["type"],  # prompt, completion
)

REQUEST_COST = Histogram(
    "agent_request_cost_usd",
    "Estimated cost per request in USD",
    buckets=[0.001, 0.005, 0.01, 0.05, 0.1, 0.5],
)

AGENT_ERRORS = Counter(
    "agent_errors_total",
    "Total errors by type",
    ["error_type"],  # tool_error, llm_error, rag_error, rate_limit, injection
)

RAG_RESULTS = Histogram(
    "agent_rag_results_count",
    "Number of RAG results returned per query",
    buckets=[0, 1, 2, 3, 5, 10],
)

AGENT_CONFIDENCE = Histogram(
    "agent_response_confidence",
    "Confidence score of agent responses",
    buckets=[0.0, 0.2, 0.4, 0.6, 0.8, 1.0],
)

FALLBACK_RATE = Counter(
    "agent_fallback_total",
    "Number of times the agent fell back to a default response",
)

ACTIVE_REQUESTS = Gauge(
    "agent_active_requests",
    "Number of currently active agent requests",
)


@contextmanager
def track_latency(step: str):
    """Context manager to track latency of a workflow step."""
    start = time.monotonic()
    try:
        yield
    finally:
        duration = time.monotonic() - start
        AGENT_LATENCY.labels(step=step).observe(duration)


def record_tokens(prompt: int, completion: int):
    """Record token usage."""
    TOKENS_USED.labels(type="prompt").inc(prompt)
    TOKENS_USED.labels(type="completion").inc(completion)


def record_cost(cost_usd: float):
    """Record estimated cost."""
    REQUEST_COST.observe(cost_usd)


def record_error(error_type: str):
    """Record an error by type."""
    AGENT_ERRORS.labels(error_type=error_type).inc()


def record_rag_results(count: int):
    """Record number of RAG results."""
    RAG_RESULTS.observe(count)


def record_confidence(confidence: float):
    """Record response confidence."""
    AGENT_CONFIDENCE.observe(confidence)
