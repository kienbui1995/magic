"""EcomOps workflow — the Tuần 2 pipeline from the Month-1 roadmap.

    Nhận tin → Classify Intent → Extract Order Info → Fetch Order Status
             → Draft Response → Guardrail Check → Send hoặc Handoff

Every external dependency (classifier, order lookup, drafting model, knowledge
lookup) is injected, so the pipeline itself has no network calls and no
platform knowledge — the same workflow runs over Zalo, Facebook, or a test
harness depending on what the caller wires in.
"""

import inspect
import logging
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable

import httpx

from .extraction import ExtractedInfo, extract
from .guardrails import GuardrailContext, GuardrailEngine, GuardrailViolation
from .intents import Intent
from .prompts import build_messages

logger = logging.getLogger("magic_packs.ecomops")

# Intents a bot should never close out on its own.
DEFAULT_HANDOFF_INTENTS = frozenset({Intent.COMPLAINT})

# Safe, non-committal fallbacks used when we refuse to send a model draft.
HANDOFF_REPLY = (
    "Dạ shop đã ghi nhận thông tin của anh/chị. "
    "Shop sẽ nhờ nhân viên kiểm tra và phản hồi lại anh/chị trong thời gian sớm nhất ạ."
)
COMPLAINT_ACK_REPLY = (
    "Dạ shop rất xin lỗi anh/chị về trải nghiệm vừa rồi. "
    "Shop đã ghi nhận và sẽ chuyển nhân viên phụ trách kiểm tra, phản hồi lại anh/chị sớm nhất ạ."
)

Drafter = Callable[[list[dict[str, str]]], Awaitable[str]]
OrderLookup = Callable[[str | None, str | None], Awaitable[dict | None]]
KnowledgeLookup = Callable[[Intent], Awaitable[list[str]]]


@dataclass
class WorkflowResult:
    """Outcome of handling one customer message.

    `reply` is always safe to send as-is. `action` says whether a human still
    needs to pick the conversation up afterwards ("handoff") or whether the bot
    fully handled it ("send").
    """

    action: str
    intent: Intent
    confidence: float
    reply: str
    extracted: ExtractedInfo = field(default_factory=ExtractedInfo)
    order: dict | None = None
    violations: list[GuardrailViolation] = field(default_factory=list)
    handoff_reason: str | None = None
    trace: list[str] = field(default_factory=list)


class EcomOpsWorkflow:
    def __init__(
        self,
        classifier: Any,
        drafter: Drafter | None = None,
        order_lookup: OrderLookup | None = None,
        knowledge_lookup: KnowledgeLookup | None = None,
        guardrails: GuardrailEngine | None = None,
        handoff_intents: frozenset[Intent] = DEFAULT_HANDOFF_INTENTS,
        min_confidence: float = 0.5,
        allowed_contacts: list[str] | None = None,
    ):
        self.classifier = classifier
        self.drafter = drafter
        self.order_lookup = order_lookup
        self.knowledge_lookup = knowledge_lookup
        self.guardrails = guardrails or GuardrailEngine()
        self.handoff_intents = handoff_intents
        self.min_confidence = min_confidence
        self.allowed_contacts = allowed_contacts or []

    async def handle(self, message: str, customer: dict | None = None) -> WorkflowResult:
        customer = customer or {}
        trace: list[str] = []

        # 1. Classify — accept both sync (rule-based) and async (LLM/hybrid) classifiers.
        intent_result = await _maybe_await(self.classifier.classify(message))
        trace.append(f"classify:{intent_result.intent.value}@{intent_result.confidence:.2f}")

        # 2. Extract order identifiers from the raw message.
        extracted = extract(message)
        if extracted.order_code or extracted.phone:
            trace.append(f"extract:code={extracted.order_code},phone={bool(extracted.phone)}")

        # 3. Look the order up when we have something to look it up by.
        order = None
        if self.order_lookup and (extracted.order_code or extracted.phone):
            try:
                order = await self.order_lookup(extracted.order_code, extracted.phone)
                trace.append("lookup:hit" if order else "lookup:miss")
            except Exception as e:
                # A flaky sheet/ERP shouldn't drop the customer's message — fall
                # through to handoff with whatever we already know.
                logger.warning("order lookup failed: %s", e)
                trace.append("lookup:error")

        def _result(action: str, reply: str, reason: str | None, violations=None) -> WorkflowResult:
            return WorkflowResult(
                action=action, intent=intent_result.intent, confidence=intent_result.confidence,
                reply=reply, extracted=extracted, order=order,
                violations=violations or [], handoff_reason=reason, trace=trace,
            )

        # Intents we never let the bot close out (complaints) short-circuit here:
        # acknowledge, then hand to a human. No model draft is involved, so
        # there's nothing for it to over-promise.
        if intent_result.intent in self.handoff_intents:
            trace.append("handoff:intent")
            return _result("handoff", COMPLAINT_ACK_REPLY, f"intent={intent_result.intent.value}")

        if intent_result.confidence < self.min_confidence:
            trace.append("handoff:low_confidence")
            return _result("handoff", HANDOFF_REPLY, "low_confidence")

        if self.drafter is None:
            trace.append("handoff:no_drafter")
            return _result("handoff", HANDOFF_REPLY, "no_drafter")

        # 4. Draft.
        knowledge = None
        if self.knowledge_lookup:
            try:
                knowledge = await self.knowledge_lookup(intent_result.intent)
            except Exception as e:
                logger.warning("knowledge lookup failed: %s", e)

        try:
            draft = await self.drafter(build_messages(intent_result.intent, message, order, knowledge))
        except Exception as e:
            logger.warning("drafting failed: %s", e)
            trace.append("handoff:draft_error")
            return _result("handoff", HANDOFF_REPLY, "draft_error")

        # 5. Guardrail the draft before it can reach the customer.
        ctx = GuardrailContext(
            customer_phone=customer.get("phone") or extracted.phone,
            customer_email=customer.get("email"),
            allowed_contacts=self.allowed_contacts,
        )
        violations = self.guardrails.check(draft, ctx)
        if GuardrailEngine.is_blocked(violations):
            trace.append(f"handoff:guardrail({','.join(v.rule for v in violations)})")
            return _result("handoff", HANDOFF_REPLY, "guardrail_blocked", violations)

        trace.append("send")
        return _result("send", draft.strip(), None, violations)


async def _maybe_await(value):
    return await value if inspect.isawaitable(value) else value


class LLMDrafter:
    """Drafts replies through MagiC's LLM gateway (`POST /api/v1/llm/chat`).

    Routed through core rather than a provider SDK so token spend lands in the
    Cost Controller alongside every other MagiC workload.
    """

    def __init__(
        self,
        magic_url: str,
        api_key: str = "",
        model: str = "",
        timeout: float = 30.0,
        max_tokens: int = 300,
        strategy: str = "best",
    ):
        self.magic_url = magic_url.rstrip("/")
        self.api_key = api_key
        self.model = model
        self.timeout = timeout
        self.max_tokens = max_tokens
        self.strategy = strategy

    async def __call__(self, messages: list[dict[str, str]]) -> str:
        headers = {"Authorization": f"Bearer {self.api_key}"} if self.api_key else {}
        payload: dict = {"messages": messages, "strategy": self.strategy, "max_tokens": self.max_tokens}
        if self.model:
            payload["model"] = self.model

        async with httpx.AsyncClient(timeout=self.timeout) as client:
            resp = await client.post(f"{self.magic_url}/api/v1/llm/chat", headers=headers, json=payload)
            resp.raise_for_status()
            return (resp.json().get("content") or "").strip()
