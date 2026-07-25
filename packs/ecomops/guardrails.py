"""Guardrail engine — inspects a draft reply BEFORE it reaches the customer.

An LLM draft is not automatically safe to send from a shop's official account:
it can promise a delivery date nobody can honour, offer a refund the shop never
approved, or echo another customer's phone number out of the context window.
Each rule here catches one of those, and anything BLOCK-severity routes the
conversation to a human instead of being sent.

Rules operate on diacritics-normalized text (see `text.normalize`) so they can't
be sidestepped by typing without dấu.
"""

import re
from dataclasses import dataclass, field
from enum import Enum
from typing import Protocol

from .extraction import PHONE_PATTERN
from .text import normalize

EMAIL_PATTERN = re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}")


class Severity(str, Enum):
    BLOCK = "block"
    WARN = "warn"


@dataclass
class GuardrailViolation:
    rule: str
    severity: Severity
    message: str
    evidence: str = ""


@dataclass
class GuardrailContext:
    """What the rules need to judge a draft in context."""

    customer_phone: str | None = None
    customer_email: str | None = None
    # The shop's own hotline/support address — legitimately quotable in a reply.
    allowed_contacts: list[str] = field(default_factory=list)
    # Set once a human has actually approved compensation for this conversation.
    compensation_approved: bool = False


class GuardrailRule(Protocol):
    name: str

    def check(self, draft: str, context: GuardrailContext) -> list[GuardrailViolation]: ...


def _phrase_hits(normalized_draft: str, phrases: list[str]) -> list[str]:
    return [p for p in phrases if p in normalized_draft]


class EmptyDraftRule:
    """A blank reply is worse than no reply — never send one."""

    name = "empty_draft"

    def check(self, draft: str, context: GuardrailContext) -> list[GuardrailViolation]:
        if draft and draft.strip():
            return []
        return [GuardrailViolation(self.name, Severity.BLOCK, "Draft trả lời rỗng.")]


class LeakedContactRule:
    """Block phone numbers / emails that belong to neither the customer nor the shop."""

    name = "leaked_contact"

    def check(self, draft: str, context: GuardrailContext) -> list[GuardrailViolation]:
        allowed_digits = {re.sub(r"\D", "", c) for c in context.allowed_contacts}
        if context.customer_phone:
            allowed_digits.add(re.sub(r"\D", "", context.customer_phone))
        allowed_emails = {c.lower() for c in context.allowed_contacts}
        if context.customer_email:
            allowed_emails.add(context.customer_email.lower())

        violations = []
        for match in PHONE_PATTERN.finditer(draft):
            phone = f"0{match.group(1)}"
            if phone not in allowed_digits and phone.lstrip("0") not in {d.lstrip("0") for d in allowed_digits}:
                violations.append(
                    GuardrailViolation(
                        self.name, Severity.BLOCK,
                        "Draft chứa số điện thoại không phải của khách hoặc của shop.",
                        evidence=phone,
                    )
                )
        for match in EMAIL_PATTERN.finditer(draft):
            if match.group(0).lower() not in allowed_emails:
                violations.append(
                    GuardrailViolation(
                        self.name, Severity.BLOCK,
                        "Draft chứa email không phải của khách hoặc của shop.",
                        evidence=match.group(0),
                    )
                )
        return violations


class DeliveryPromiseRule:
    """Block hard delivery guarantees — the shop doesn't control the carrier."""

    name = "delivery_promise"

    PHRASES = [
        "chac chan giao", "chac chan nhan", "chac chan den", "cam ket giao",
        "dam bao giao", "dam bao nhan", "chac chan ngay mai", "100% giao",
        "giao dung ngay", "nhat dinh se giao",
    ]

    def check(self, draft: str, context: GuardrailContext) -> list[GuardrailViolation]:
        hits = _phrase_hits(normalize(draft), self.PHRASES)
        return [
            GuardrailViolation(
                self.name, Severity.BLOCK,
                "Draft hứa chắc chắn về thời gian giao hàng.",
                evidence=hit,
            )
            for hit in hits
        ]


class CompensationPromiseRule:
    """Block refunds/vouchers/free shipping the shop hasn't approved."""

    name = "compensation_promise"

    PHRASES = [
        "hoan tien 100", "hoan tien ngay", "hoan lai toan bo", "tang voucher",
        "tang ma giam gia", "boi thuong", "den bu", "mien phi ship cho anh",
        "mien phi ship cho chi", "giam gia cho anh", "giam gia cho chi",
    ]

    def check(self, draft: str, context: GuardrailContext) -> list[GuardrailViolation]:
        if context.compensation_approved:
            return []
        hits = _phrase_hits(normalize(draft), self.PHRASES)
        return [
            GuardrailViolation(
                self.name, Severity.BLOCK,
                "Draft tự hứa đền bù/hoàn tiền/giảm giá khi chưa được duyệt.",
                evidence=hit,
            )
            for hit in hits
        ]


class InternalLeakRule:
    """Block prompt/system/credential text that leaked into the draft."""

    name = "internal_leak"

    PHRASES = [
        "system prompt", "system:", "internal note", "[internal]", "nguyen tac bat buoc",
        "ban la nhan vien cham soc", "api key", "bearer ", "sk-", "todo:",
    ]

    def check(self, draft: str, context: GuardrailContext) -> list[GuardrailViolation]:
        hits = _phrase_hits(normalize(draft), self.PHRASES)
        return [
            GuardrailViolation(
                self.name, Severity.BLOCK,
                "Draft lộ nội dung nội bộ / prompt hệ thống.",
                evidence=hit,
            )
            for hit in hits
        ]


DEFAULT_RULES: list[GuardrailRule] = [
    EmptyDraftRule(),
    LeakedContactRule(),
    DeliveryPromiseRule(),
    CompensationPromiseRule(),
    InternalLeakRule(),
]


class GuardrailEngine:
    def __init__(self, rules: list[GuardrailRule] | None = None):
        self.rules = rules if rules is not None else list(DEFAULT_RULES)

    def check(self, draft: str, context: GuardrailContext | None = None) -> list[GuardrailViolation]:
        ctx = context or GuardrailContext()
        violations: list[GuardrailViolation] = []
        for rule in self.rules:
            violations.extend(rule.check(draft, ctx))
        return violations

    @staticmethod
    def is_blocked(violations: list[GuardrailViolation]) -> bool:
        return any(v.severity is Severity.BLOCK for v in violations)
