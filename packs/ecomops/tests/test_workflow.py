"""End-to-end tests for the EcomOps pipeline (no network — deps are injected)."""

import httpx
import pytest
import respx

from ecomops.guardrails import GuardrailEngine
from ecomops.intents import Intent, IntentResult, RuleBasedIntentClassifier
from ecomops.workflow import (
    COMPLAINT_ACK_REPLY,
    HANDOFF_REPLY,
    EcomOpsWorkflow,
    LLMDrafter,
)

ORDER = {"id": "DH12345", "status": "shipping", "total_amount": 250000}


class drafter_returning:
    """Async drafter stub that records the prompt it was handed."""

    def __init__(self, text: str):
        self.text = text
        self.messages: list[dict[str, str]] | None = None

    async def __call__(self, messages):
        self.messages = messages
        return self.text


class lookup_returning:
    """Async order-lookup stub that records the identifiers it was called with."""

    def __init__(self, order):
        self.order = order
        self.called_with: tuple | None = None

    async def __call__(self, order_code, phone):
        self.called_with = (order_code, phone)
        return self.order


class StubClassifier:
    """Sync classifier stub — also proves the workflow accepts non-async ones."""

    def __init__(self, intent: Intent, confidence: float = 0.9):
        self.result = IntentResult(intent, confidence)

    def classify(self, message):
        return self.result


class AsyncStubClassifier:
    def __init__(self, intent: Intent, confidence: float = 0.9):
        self.result = IntentResult(intent, confidence)

    async def classify(self, message):
        return self.result


# ---- happy path ----


async def test_confident_intent_with_clean_draft_is_sent():
    drafter = drafter_returning("Dạ đơn của anh/chị đang được giao ạ.")
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.ORDER_STATUS),
        drafter=drafter,
        order_lookup=lookup_returning(ORDER),
    )

    result = await wf.handle("đơn #DH12345 tới đâu rồi")

    assert result.action == "send"
    assert result.reply == "Dạ đơn của anh/chị đang được giao ạ."
    assert result.order == ORDER
    assert result.violations == []


async def test_async_classifier_is_also_supported():
    wf = EcomOpsWorkflow(
        classifier=AsyncStubClassifier(Intent.SHIPPING_FEE),
        drafter=drafter_returning("Dạ phí ship 30k ạ."),
    )
    assert (await wf.handle("phí ship bao nhiêu")).action == "send"


async def test_real_rule_classifier_end_to_end():
    wf = EcomOpsWorkflow(
        classifier=RuleBasedIntentClassifier(),
        drafter=drafter_returning("Dạ phí ship về Đà Nẵng là 30.000đ ạ."),
    )
    result = await wf.handle("phí ship về Đà Nẵng bao nhiêu shop, phí vận chuyển ấy")
    assert result.intent is Intent.SHIPPING_FEE
    assert result.action == "send"


# ---- order lookup ----


async def test_lookup_receives_extracted_identifiers():
    lookup = lookup_returning(ORDER)
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.ORDER_STATUS),
        drafter=drafter_returning("ok ạ"),
        order_lookup=lookup,
    )

    await wf.handle("đơn DH-556677 của em, sdt 0901234567")

    assert lookup.called_with == ("DH-556677", "0901234567")


async def test_lookup_is_skipped_without_identifiers():
    lookup = lookup_returning(ORDER)
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.ORDER_STATUS),
        drafter=drafter_returning("ok ạ"),
        order_lookup=lookup,
    )

    await wf.handle("đơn của em sao rồi shop")

    assert lookup.called_with is None


async def test_lookup_failure_does_not_drop_the_message():
    async def failing_lookup(order_code, phone):
        raise RuntimeError("sheet unavailable")

    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.ORDER_STATUS),
        drafter=drafter_returning("Dạ shop kiểm tra giúp anh/chị ạ."),
        order_lookup=failing_lookup,
    )

    result = await wf.handle("đơn #DH12345 đâu rồi")

    assert result.action == "send"  # degraded to a data-less draft, not a crash
    assert result.order is None
    assert "lookup:error" in result.trace


async def test_order_data_reaches_the_prompt():
    drafter = drafter_returning("ok ạ")
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.ORDER_STATUS),
        drafter=drafter,
        order_lookup=lookup_returning(ORDER),
    )

    await wf.handle("đơn #DH12345 đâu")

    assert "DH12345" in drafter.messages[-1]["content"]


async def test_knowledge_reaches_the_prompt():
    async def knowledge(intent):
        return ["Đổi trả trong 7 ngày kể từ khi nhận hàng."]

    drafter = drafter_returning("ok ạ")
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.RETURN_EXCHANGE),
        drafter=drafter,
        knowledge_lookup=knowledge,
    )

    await wf.handle("em muốn đổi size")

    assert "Đổi trả trong 7 ngày" in drafter.messages[-1]["content"]


# ---- handoff paths ----


async def test_complaint_always_goes_to_a_human():
    drafter = drafter_returning("shop hoàn tiền ngay cho anh/chị")
    wf = EcomOpsWorkflow(classifier=StubClassifier(Intent.COMPLAINT), drafter=drafter)

    result = await wf.handle("hàng lỗi, tôi muốn khiếu nại")

    assert result.action == "handoff"
    assert result.reply == COMPLAINT_ACK_REPLY
    assert drafter.messages is None  # never asked the model to improvise here


async def test_low_confidence_goes_to_a_human():
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.OTHER, confidence=0.0),
        drafter=drafter_returning("..."),
    )

    result = await wf.handle("???")

    assert result.action == "handoff"
    assert result.handoff_reason == "low_confidence"
    assert result.reply == HANDOFF_REPLY


async def test_missing_drafter_goes_to_a_human():
    wf = EcomOpsWorkflow(classifier=StubClassifier(Intent.ORDER_STATUS), drafter=None)
    result = await wf.handle("đơn #DH12345 đâu")
    assert result.action == "handoff"
    assert result.handoff_reason == "no_drafter"


async def test_drafting_error_goes_to_a_human():
    async def failing_drafter(messages):
        raise RuntimeError("llm down")

    wf = EcomOpsWorkflow(classifier=StubClassifier(Intent.ORDER_STATUS), drafter=failing_drafter)

    result = await wf.handle("đơn #DH12345 đâu")

    assert result.action == "handoff"
    assert result.handoff_reason == "draft_error"


async def test_guardrail_blocked_draft_is_never_sent_to_the_customer():
    bad_draft = "Shop chắc chắn giao ngày mai và hoàn tiền 100% cho anh/chị ạ."
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.ORDER_STATUS),
        drafter=drafter_returning(bad_draft),
        guardrails=GuardrailEngine(),
    )

    result = await wf.handle("đơn #DH12345 bao giờ giao")

    assert result.action == "handoff"
    assert result.handoff_reason == "guardrail_blocked"
    assert result.reply == HANDOFF_REPLY
    assert bad_draft not in result.reply
    assert {"delivery_promise", "compensation_promise"} <= {v.rule for v in result.violations}


async def test_customer_own_phone_in_draft_is_not_treated_as_a_leak():
    draft = "Dạ shop giao tới số 0901234567 của anh/chị ạ."
    wf = EcomOpsWorkflow(classifier=StubClassifier(Intent.ORDER_STATUS), drafter=drafter_returning(draft))

    result = await wf.handle("đơn của em sdt 0901234567", customer={"phone": "0901234567"})

    assert result.action == "send"


async def test_trace_records_the_pipeline_steps():
    wf = EcomOpsWorkflow(
        classifier=StubClassifier(Intent.ORDER_STATUS),
        drafter=drafter_returning("ok ạ"),
        order_lookup=lookup_returning(ORDER),
    )

    result = await wf.handle("đơn #DH12345 đâu")

    assert any(t.startswith("classify:order_status") for t in result.trace)
    assert "lookup:hit" in result.trace
    assert "send" in result.trace


# ---- LLMDrafter ----


@respx.mock
async def test_llm_drafter_calls_magic_gateway():
    route = respx.post("http://magic:8080/api/v1/llm/chat").mock(
        return_value=httpx.Response(200, json={"content": "  Dạ shop đã nhận đơn ạ.  "})
    )

    text = await LLMDrafter("http://magic:8080", api_key="k")([{"role": "user", "content": "hi"}])

    assert text == "Dạ shop đã nhận đơn ạ."
    assert route.calls.last.request.headers["Authorization"] == "Bearer k"


@respx.mock
async def test_llm_drafter_raises_on_gateway_error():
    respx.post("http://magic:8080/api/v1/llm/chat").mock(return_value=httpx.Response(502, json={}))
    with pytest.raises(httpx.HTTPStatusError):
        await LLMDrafter("http://magic:8080")([{"role": "user", "content": "hi"}])
