"""Tests for the planner node."""

from app.models import AgentState, CustomerProfile, Transaction
from app.nodes.planner import plan_steps


class TestPlanSteps:
    def test_full_plan_with_transactions(self):
        state = AgentState(
            customer_id="cust-123",
            profile=CustomerProfile(
                customer_id="cust-123",
                name="Test",
                document="12345",
                segment="middle",
            ),
            transactions=[
                Transaction(id="1", date="2025-01-01", amount=100, type="credit", category="rev"),
            ],
        )

        result = plan_steps(state)
        assert "retrieve_knowledge" in result["plan"]
        assert "analyze_financials" in result["plan"]

    def test_plan_without_transactions(self):
        state = AgentState(
            customer_id="cust-123",
            profile=CustomerProfile(
                customer_id="cust-123",
                name="Test",
                document="12345",
                segment="middle",
            ),
        )

        result = plan_steps(state)
        assert "retrieve_knowledge" in result["plan"]
        assert "analyze_financials" not in result["plan"]
