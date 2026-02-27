"""
Security & Governance module.

Implements:
- Input sanitization
- Prompt injection detection
- PII redaction
- Rate limiting
- Cost control
"""

from __future__ import annotations

import re
from collections import defaultdict
from time import time

import structlog

from app.config import settings

logger = structlog.get_logger()

# Known prompt injection patterns
INJECTION_PATTERNS = [
    r"ignore\s+(all\s+)?previous\s+instructions",
    r"ignore\s+the\s+above",
    r"disregard\s+(all\s+)?prior",
    r"you\s+are\s+now\s+a",
    r"act\s+as\s+if",
    r"pretend\s+(you\s+are|to\s+be)",
    r"system\s*:\s*",
    r"<\s*/?system\s*>",
    r"###\s*instruction",
    r"OVERRIDE",
]

# PII patterns for redaction
PII_PATTERNS = {
    "cpf": (r"\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b", "[CPF_REDACTED]"),
    "cnpj": (r"\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b", "[CNPJ_REDACTED]"),
    "email": (r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b", "[EMAIL_REDACTED]"),
    "phone": (r"\b\(?\d{2}\)?\s*\d{4,5}-?\d{4}\b", "[PHONE_REDACTED]"),
    "card_number": (r"\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b", "[CARD_REDACTED]"),
}


def sanitize_input(text: str) -> str:
    """
    Sanitize user input:
    1. Trim and limit length
    2. Remove control characters
    3. Check for prompt injection
    """
    if not text:
        return text

    # Length limit
    text = text[: settings.max_input_length]

    # Remove control characters (keep newlines, tabs)
    text = re.sub(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]", "", text)

    return text.strip()


def detect_prompt_injection(text: str) -> bool:
    """
    Detect potential prompt injection attempts.
    Returns True if injection is detected.
    """
    lower_text = text.lower()
    for pattern in INJECTION_PATTERNS:
        if re.search(pattern, lower_text, re.IGNORECASE):
            logger.warning("security.prompt_injection_detected", pattern=pattern)
            return True
    return False


def redact_pii(text: str) -> str:
    """Redact personally identifiable information from text."""
    result = text
    for pii_type, (pattern, replacement) in PII_PATTERNS.items():
        matches = re.findall(pattern, result)
        if matches:
            logger.info("security.pii_redacted", type=pii_type, count=len(matches))
            result = re.sub(pattern, replacement, result)
    return result


class RateLimiter:
    """Simple in-memory sliding window rate limiter per customer."""

    def __init__(self, max_requests: int = 100, window_seconds: int = 3600):
        self.max_requests = max_requests
        self.window_seconds = window_seconds
        self._requests: dict[str, list[float]] = defaultdict(list)

    def is_allowed(self, customer_id: str) -> bool:
        """Check if the customer is within rate limits."""
        now = time()
        window_start = now - self.window_seconds

        # Clean old entries
        self._requests[customer_id] = [
            ts for ts in self._requests[customer_id] if ts > window_start
        ]

        if len(self._requests[customer_id]) >= self.max_requests:
            logger.warning("security.rate_limit_exceeded", customer_id=customer_id)
            return False

        self._requests[customer_id].append(now)
        return True


class CostController:
    """Tracks and limits LLM costs per customer."""

    def __init__(self, max_daily_cost_per_user: float = 1.0):
        self.max_daily_cost = max_daily_cost_per_user
        self._daily_costs: dict[str, float] = defaultdict(float)
        self._last_reset: float = time()

    def estimate_cost(self, prompt_tokens: int, completion_tokens: int) -> float:
        """Estimate the cost of an LLM call in USD."""
        prompt_cost = (prompt_tokens / 1000) * settings.cost_per_1k_prompt_tokens
        completion_cost = (completion_tokens / 1000) * settings.cost_per_1k_completion_tokens
        return prompt_cost + completion_cost

    def record_and_check(self, customer_id: str, cost: float) -> bool:
        """Record cost and return True if within daily limit."""
        self._maybe_reset()
        self._daily_costs[customer_id] += cost

        if self._daily_costs[customer_id] > self.max_daily_cost:
            logger.warning(
                "security.cost_limit_exceeded",
                customer_id=customer_id,
                total_cost=self._daily_costs[customer_id],
            )
            return False
        return True

    def _maybe_reset(self):
        """Reset daily counters every 24h."""
        now = time()
        if now - self._last_reset > 86400:
            self._daily_costs.clear()
            self._last_reset = now


# Module-level instances
rate_limiter = RateLimiter(max_requests=settings.rate_limit_per_user)
cost_controller = CostController()
