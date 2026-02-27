"""
Retriever node — performs RAG (Retrieval-Augmented Generation)
to fetch relevant knowledge from the vector store.
"""

from __future__ import annotations

import structlog

from app.models import AgentState
from app.rag.retriever import search_knowledge_base

logger = structlog.get_logger()


def retrieve_knowledge(state: AgentState) -> dict:
    """
    Build a semantic query from customer context and retrieve
    relevant documents from the knowledge base.
    """
    logger.info("retriever.executing", customer_id=state.customer_id)

    # Build a rich query from customer context
    query_parts = []

    if state.profile:
        query_parts.append(f"Cliente PJ segmento {state.profile.segment}")
        if state.profile.monthly_revenue > 0:
            query_parts.append(f"faturamento mensal R${state.profile.monthly_revenue:,.2f}")
        if state.profile.credit_score > 0:
            query_parts.append(f"score de crédito {state.profile.credit_score}")

    if state.query:
        query_parts.append(state.query)
    else:
        query_parts.append("recomendações financeiras para empresa")

    query = ". ".join(query_parts)

    try:
        results = search_knowledge_base(query, top_k=3)
        context_text = "\n\n---\n\n".join([doc.page_content for doc in results])
        sources = [doc.metadata.get("source", "unknown") for doc in results]

        logger.info(
            "retriever.success",
            customer_id=state.customer_id,
            num_results=len(results),
            sources=sources,
        )

        return {
            "retrieved_context": context_text,
            "rag_sources": sources,
            "tools_executed": state.tools_executed + ["rag_retrieval"],
        }

    except Exception as e:
        logger.error("retriever.failed", error=str(e), customer_id=state.customer_id)
        return {
            "retrieved_context": "",
            "rag_sources": [],
            "errors": state.errors + [f"RAG retrieval failed: {e}"],
            "tools_executed": state.tools_executed + ["rag_retrieval_failed"],
        }
