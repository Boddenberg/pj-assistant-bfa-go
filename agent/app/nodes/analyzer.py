"""
Analyzer node — analyzes financial data (transactions, profile)
and produces structured insights.
"""

from __future__ import annotations

import structlog

from app.models import AgentState, Transaction

logger = structlog.get_logger()


def analyze_financials(state: AgentState) -> dict:
    """
    Analyze transactions and produce financial insights.
    This is a deterministic tool — no LLM call needed.
    """
    logger.info("analyzer.executing", customer_id=state.customer_id)

    transactions = state.transactions
    if not transactions:
        return {
            "tool_results": {**state.tool_results, "financial_analysis": "No transaction data available."},
            "tools_executed": state.tools_executed + ["financial_analysis"],
        }

    analysis = _compute_analysis(transactions)

    logger.info("analyzer.success", customer_id=state.customer_id)

    return {
        "tool_results": {**state.tool_results, "financial_analysis": analysis},
        "tools_executed": state.tools_executed + ["financial_analysis"],
    }


def _compute_analysis(transactions: list[Transaction]) -> str:
    """Compute financial metrics from raw transactions."""
    total_income = sum(t.amount for t in transactions if t.amount > 0)
    total_expenses = abs(sum(t.amount for t in transactions if t.amount < 0))
    net_cashflow = total_income - total_expenses

    # Category breakdown
    categories: dict[str, float] = {}
    for t in transactions:
        cat = t.category or "uncategorized"
        categories[cat] = categories.get(cat, 0) + abs(t.amount)

    top_categories = sorted(categories.items(), key=lambda x: x[1], reverse=True)[:5]

    lines = [
        f"📊 Resumo Financeiro:",
        f"  • Receita total: R${total_income:,.2f}",
        f"  • Despesas totais: R${total_expenses:,.2f}",
        f"  • Fluxo de caixa líquido: R${net_cashflow:,.2f}",
        f"  • Número de transações: {len(transactions)}",
        f"",
        f"📂 Top categorias por volume:",
    ]

    for cat, amount in top_categories:
        lines.append(f"  • {cat}: R${amount:,.2f}")

    # Health indicators
    if net_cashflow > 0:
        lines.append(f"\n✅ Fluxo de caixa positivo — empresa saudável financeiramente.")
    else:
        lines.append(f"\n⚠️ Fluxo de caixa negativo — atenção ao capital de giro.")

    if total_income > 0:
        expense_ratio = total_expenses / total_income
        lines.append(f"  • Razão despesas/receita: {expense_ratio:.1%}")

    return "\n".join(lines)
