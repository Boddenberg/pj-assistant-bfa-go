"""
LangGraph-based multi-step agent workflow.

The agent follows this graph:

    [START]
       │
       ▼
    planner ──► decides which steps are needed
       │
       ▼
    retriever ──► RAG: fetch relevant knowledge
       │
       ▼
    analyzer ──► analyze financial data with tools
       │
       ▼
    synthesizer ──► generate final recommendation
       │
       ▼
    [END]
"""

from __future__ import annotations

import structlog
from langgraph.graph import END, StateGraph

from app.models import AgentState
from app.nodes.analyzer import analyze_financials
from app.nodes.planner import plan_steps
from app.nodes.retriever import retrieve_knowledge
from app.nodes.synthesizer import synthesize_response

logger = structlog.get_logger()


def should_retrieve(state: AgentState) -> str:
    """Conditional edge: skip RAG if the plan doesn't require it."""
    if "retrieve_knowledge" in state.plan:
        return "retriever"
    return "analyzer"


def should_analyze(state: AgentState) -> str:
    """Conditional edge: skip analysis if not required."""
    if "analyze_financials" in state.plan:
        return "analyzer"
    return "synthesizer"


def build_agent_graph() -> StateGraph:
    """Builds and compiles the LangGraph workflow."""

    workflow = StateGraph(AgentState)

    # --- Nodes ---
    workflow.add_node("planner", plan_steps)
    workflow.add_node("retriever", retrieve_knowledge)
    workflow.add_node("analyzer", analyze_financials)
    workflow.add_node("synthesizer", synthesize_response)

    # --- Edges ---
    workflow.set_entry_point("planner")

    workflow.add_conditional_edges(
        "planner",
        should_retrieve,
        {
            "retriever": "retriever",
            "analyzer": "analyzer",
        },
    )

    workflow.add_conditional_edges(
        "retriever",
        should_analyze,
        {
            "analyzer": "analyzer",
            "synthesizer": "synthesizer",
        },
    )

    workflow.add_edge("analyzer", "synthesizer")
    workflow.add_edge("synthesizer", END)

    return workflow.compile()


# Pre-built graph instance
agent_graph = build_agent_graph()
