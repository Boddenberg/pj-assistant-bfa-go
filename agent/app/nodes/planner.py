"""
Planner node — decides which steps the agent should execute
based on the incoming request context.
"""

from __future__ import annotations

import structlog

from app.models import AgentState

logger = structlog.get_logger()


def plan_steps(state: AgentState) -> dict:
    """
    Analyze the request and decide which workflow steps are needed.

    Always includes:
    - retrieve_knowledge: fetch relevant docs via RAG
    - analyze_financials: analyze transaction patterns
    - synthesize: generate final response (implicit, always runs)
    """
    logger.info("planner.executing", customer_id=state.customer_id)

    plan = []

    # Always retrieve knowledge for context enrichment
    plan.append("retrieve_knowledge")

    # Analyze financials if we have transaction data
    if state.transactions:
        plan.append("analyze_financials")

    logger.info("planner.plan_created", plan=plan, customer_id=state.customer_id)

    return {"plan": plan}
