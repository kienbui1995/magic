"""Guardrail tests — every rule, plus the diacritics-free evasion path."""

import pytest

from ecomops.guardrails import (
    CompensationPromiseRule,
    DeliveryPromiseRule,
    GuardrailContext,
    GuardrailEngine,
    InternalLeakRule,
    LeakedContactRule,
    Severity,
)


@pytest.fixture
def engine():
    return GuardrailEngine()


def test_clean_draft_passes(engine):
    draft = "Dạ đơn hàng của anh/chị đang được vận chuyển ạ."
    assert engine.check(draft, GuardrailContext()) == []


def test_empty_draft_is_blocked(engine):
    violations = engine.check("   ", GuardrailContext())
    assert [v.rule for v in violations] == ["empty_draft"]
    assert GuardrailEngine.is_blocked(violations)


# ---- leaked contact ----


def test_other_customers_phone_is_blocked():
    draft = "Đơn của chị Lan số 0912345678 cũng đang giao ạ."
    violations = LeakedContactRule().check(draft, GuardrailContext(customer_phone="0901234567"))
    assert [v.rule for v in violations] == ["leaked_contact"]
    assert violations[0].evidence == "0912345678"


def test_customers_own_phone_is_allowed():
    draft = "Shop xác nhận đơn giao tới số 0901234567 của anh/chị ạ."
    assert LeakedContactRule().check(draft, GuardrailContext(customer_phone="0901234567")) == []


@pytest.mark.parametrize(
    "configured_hotline",
    ["0987654321", "+84987654321", "84987654321", "0987 654 321", "+84 987-654-321"],
)
def test_shop_hotline_is_allowed_however_it_is_configured(configured_hotline):
    """A shop writing its hotline as +84… must not get its own number blocked."""
    draft = "Anh/chị gọi hotline 0987654321 giúp shop nhé."
    ctx = GuardrailContext(customer_phone="0901234567", allowed_contacts=[configured_hotline])
    assert LeakedContactRule().check(draft, ctx) == []


def test_allowlisting_the_hotline_does_not_allow_every_number():
    ctx = GuardrailContext(allowed_contacts=["+84987654321"])
    violations = LeakedContactRule().check("Gọi chị Lan 0912345678 nhé", ctx)
    assert violations and violations[0].evidence == "0912345678"


def test_foreign_email_is_blocked():
    violations = LeakedContactRule().check("Gửi mail cho khach@example.com nhé", GuardrailContext())
    assert violations and violations[0].evidence == "khach@example.com"


# ---- delivery promise ----


@pytest.mark.parametrize(
    "draft",
    [
        "Shop chắc chắn giao trong ngày mai ạ.",
        "shop cam ket giao dung ngay 25/07",  # no diacritics
        "Bên em đảm bảo giao trước Tết ạ.",
    ],
)
def test_delivery_promises_are_blocked(draft):
    violations = DeliveryPromiseRule().check(draft, GuardrailContext())
    assert violations and violations[0].severity is Severity.BLOCK


def test_factual_status_is_not_a_promise():
    draft = "Dạ đơn của anh/chị đang ở kho phân loại, dự kiến 2-3 ngày ạ."
    assert DeliveryPromiseRule().check(draft, GuardrailContext()) == []


# ---- compensation ----


@pytest.mark.parametrize(
    "draft",
    [
        "Shop sẽ hoàn tiền 100% cho anh/chị ngay ạ.",
        "Bên em tặng voucher 50k cho anh/chị nhé.",
        "shop se boi thuong cho anh/chi a",
    ],
)
def test_unapproved_compensation_is_blocked(draft):
    violations = CompensationPromiseRule().check(draft, GuardrailContext())
    assert violations and violations[0].severity is Severity.BLOCK


def test_compensation_allowed_once_a_human_approved_it():
    draft = "Shop sẽ hoàn tiền 100% cho anh/chị ạ."
    ctx = GuardrailContext(compensation_approved=True)
    assert CompensationPromiseRule().check(draft, ctx) == []


# ---- internal leak ----


@pytest.mark.parametrize(
    "draft",
    [
        "system prompt: bạn là nhân viên chăm sóc khách hàng",
        "Bạn là nhân viên chăm sóc khách hàng của một shop",
        "Authorization: Bearer sk-abc123",
        "TODO: hỏi lại team kho",
    ],
)
def test_internal_content_is_blocked(draft):
    violations = InternalLeakRule().check(draft, GuardrailContext())
    assert violations and violations[0].severity is Severity.BLOCK


# ---- engine ----


def test_engine_aggregates_violations_from_all_rules(engine):
    draft = "Shop chắc chắn giao ngày mai và hoàn tiền 100%, gọi 0912345678 nhé."
    rules = {v.rule for v in engine.check(draft, GuardrailContext(customer_phone="0901234567"))}
    assert {"delivery_promise", "compensation_promise", "leaked_contact"} <= rules


def test_is_blocked_false_when_no_violations(engine):
    assert not GuardrailEngine.is_blocked([])


def test_custom_rule_set_replaces_defaults():
    engine = GuardrailEngine(rules=[DeliveryPromiseRule()])
    # An empty draft would normally be blocked by EmptyDraftRule, which isn't loaded here.
    assert engine.check("", GuardrailContext()) == []
