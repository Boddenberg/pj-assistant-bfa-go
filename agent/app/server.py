"""
FastAPI server exposing the AI Agent as an HTTP API.

This is the entry point called by the BFA (Go) service.
"""

from __future__ import annotations

import structlog
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from prometheus_client import generate_latest, CONTENT_TYPE_LATEST

from app.config import settings
from app.graph import agent_graph
from app.models import AgentRequest, AgentResponse, AgentState, TokenUsage
from app.observability import (
    ACTIVE_REQUESTS,
    FALLBACK_RATE,
    record_confidence,
    record_cost,
    record_error,
    record_rag_results,
    record_tokens,
    track_latency,
)
from app.security import (
    cost_controller,
    detect_prompt_injection,
    rate_limiter,
    redact_pii,
    sanitize_input,
)

logger = structlog.get_logger()

app = FastAPI(
    title="PJ Assistant Agent",
    description="AI Agent for Itaú PJ Assistant (BFA)",
    version="1.0.0",
)


@app.post("/v1/agent/invoke", response_model=AgentResponse)
async def invoke_agent(request: AgentRequest) -> AgentResponse:
    """
    Main endpoint — invoked by the BFA to get AI-powered recommendations.
    """
    customer_id = request.customer_id
    logger.info("agent.request_received", customer_id=customer_id)
    ACTIVE_REQUESTS.inc()

    try:
        # --- Security checks ---
        if not rate_limiter.is_allowed(customer_id):
            record_error("rate_limit")
            raise HTTPException(status_code=429, detail="Rate limit exceeded")

        if request.query and detect_prompt_injection(request.query):
            record_error("injection")
            raise HTTPException(status_code=400, detail="Invalid input detected")

        # Sanitize query
        sanitized_query = sanitize_input(request.query or "")

        # --- Build initial agent state ---
        initial_state = AgentState(
            customer_id=customer_id,
            profile=request.profile,
            transactions=request.transactions,
            query=sanitized_query,
        )

        # --- Execute agent graph ---
        with track_latency("total"):
            result = agent_graph.invoke(initial_state)

        # --- Process result ---
        # LangGraph returns a dict; extract what we need
        if isinstance(result, dict):
            answer = result.get("answer", "")
            reasoning = result.get("reasoning", "")
            confidence = result.get("confidence", 0.0)
            sources = result.get("rag_sources", [])
            tools_executed = result.get("tools_executed", [])
            token_usage_data = result.get("token_usage", {})
        else:
            answer = result.answer
            reasoning = result.reasoning
            confidence = result.confidence
            sources = result.rag_sources
            tools_executed = result.tools_executed
            token_usage_data = result.token_usage

        # Build token usage
        if isinstance(token_usage_data, dict):
            token_usage = TokenUsage(**token_usage_data)
        elif isinstance(token_usage_data, TokenUsage):
            token_usage = token_usage_data
        else:
            token_usage = TokenUsage()

        # --- Record metrics ---
        record_tokens(token_usage.prompt_tokens, token_usage.completion_tokens)
        record_confidence(confidence)
        record_rag_results(len(sources))

        estimated_cost = cost_controller.estimate_cost(
            token_usage.prompt_tokens, token_usage.completion_tokens
        )
        record_cost(estimated_cost)
        cost_controller.record_and_check(customer_id, estimated_cost)

        # Redact PII from the answer
        answer = redact_pii(answer)

        # Fallback detection
        if confidence < 0.3 or not answer:
            FALLBACK_RATE.inc()
            if not answer:
                answer = (
                    "Não foi possível gerar uma recomendação completa. "
                    "Por favor, consulte seu gerente de relacionamento."
                )

        response = AgentResponse(
            answer=answer,
            reasoning=reasoning,
            sources=sources,
            confidence=confidence,
            tokens_used=token_usage,
            tools_executed=tools_executed,
        )

        logger.info(
            "agent.request_completed",
            customer_id=customer_id,
            confidence=confidence,
            tokens=token_usage.total_tokens,
            cost_usd=estimated_cost,
        )

        return response

    except HTTPException:
        raise
    except Exception as e:
        logger.error("agent.request_failed", customer_id=customer_id, error=str(e))
        record_error("unhandled")
        raise HTTPException(status_code=500, detail="Internal agent error")
    finally:
        ACTIVE_REQUESTS.dec()


@app.get("/healthz")
async def healthz():
    return {"status": "alive"}


@app.get("/readyz")
async def readyz():
    return {"status": "ready"}


@app.get("/metrics")
async def metrics():
    """Expose Prometheus metrics."""
    return JSONResponse(
        content=generate_latest().decode("utf-8"),
        media_type=CONTENT_TYPE_LATEST,
    )
