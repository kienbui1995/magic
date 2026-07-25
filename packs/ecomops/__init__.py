"""EcomOps Pack — customer-service workflow for Vietnamese online shops.

Open-source reference implementation of the EcomOps workflow: classify an
inbound message, look the order up, draft a reply, and guardrail it before it
reaches the customer.
"""

from .guardrails import GuardrailContext, GuardrailEngine, GuardrailViolation, Severity
from .intents import (
    HybridIntentClassifier,
    Intent,
    IntentResult,
    LLMIntentClassifier,
    RuleBasedIntentClassifier,
)
from .workflow import EcomOpsWorkflow, LLMDrafter, WorkflowResult

__all__ = [
    "Intent",
    "IntentResult",
    "RuleBasedIntentClassifier",
    "LLMIntentClassifier",
    "HybridIntentClassifier",
    "GuardrailEngine",
    "GuardrailContext",
    "GuardrailViolation",
    "Severity",
    "EcomOpsWorkflow",
    "LLMDrafter",
    "WorkflowResult",
]
