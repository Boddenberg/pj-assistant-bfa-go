"""Tests for the financial analyzer node."""

from app.models import Transaction
from app.nodes.analyzer import analyze_financials, AgentState


class TestAnalyzeFinancials:
    def test_with_transactions(self):
        state = AgentState(
            customer_id="cust-123",
            transactions=[
                Transaction(id="1", date="2025-01-01", amount=5000, type="credit", category="revenue"),
                Transaction(id="2", date="2025-01-02", amount=-2000, type="debit", category="supplier"),
                Transaction(id="3", date="2025-01-03", amount=-500, type="debit", category="utilities"),
            ],
        )

        result = analyze_financials(state)
        analysis = result["tool_results"]["financial_analysis"]

        assert "5,000.00" in analysis or "5000" in analysis
        assert "financial_analysis" in result["tools_executed"]

    def test_without_transactions(self):
        state = AgentState(customer_id="cust-123", transactions=[])

        result = analyze_financials(state)
        assert "No transaction data" in result["tool_results"]["financial_analysis"]

    def test_negative_cashflow_warning(self):
        state = AgentState(
            customer_id="cust-123",
            transactions=[
                Transaction(id="1", date="2025-01-01", amount=1000, type="credit", category="revenue"),
                Transaction(id="2", date="2025-01-02", amount=-3000, type="debit", category="rent"),
            ],
        )

        result = analyze_financials(state)
        analysis = result["tool_results"]["financial_analysis"]
        assert "negativo" in analysis or "⚠️" in analysis
