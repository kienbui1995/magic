"""Intent classification tests — including the diacritics-free spellings real
Vietnamese customers actually type."""

import httpx
import pytest
import respx

from ecomops.intents import (
    HybridIntentClassifier,
    Intent,
    LLMIntentClassifier,
    RuleBasedIntentClassifier,
)

MAGIC_URL = "http://magic:8080"
CHAT_URL = f"{MAGIC_URL}/api/v1/llm/chat"


@pytest.fixture
def rules():
    return RuleBasedIntentClassifier()


@pytest.mark.parametrize(
    "message,expected",
    [
        ("Đơn hàng của em tới đâu rồi ạ?", Intent.ORDER_STATUS),
        ("don hang cua em toi dau roi a", Intent.ORDER_STATUS),  # no diacritics
        ("Khi nào em nhận được hàng vậy shop", Intent.ORDER_STATUS),
        ("Phí ship về Đà Nẵng bao nhiêu ạ?", Intent.SHIPPING_FEE),
        ("phi van chuyen la bao nhieu", Intent.SHIPPING_FEE),
        ("Em muốn đổi size áo này được không", Intent.RETURN_EXCHANGE),
        ("shop giao sai màu rồi, bực quá", Intent.COMPLAINT),
        ("hang loi, em muon khieu nai", Intent.COMPLAINT),
        ("Áo này còn size M không shop?", Intent.PRODUCT_INFO),
        ("Xin chào shop", Intent.GREETING),
        ("ừm", Intent.OTHER),
    ],
)
def test_rule_based_classification(rules, message, expected):
    assert rules.classify(message).intent is expected


def test_diacritic_and_plain_spelling_agree(rules):
    with_marks = rules.classify("Đơn hàng của tôi đâu rồi?")
    without_marks = rules.classify("Don hang cua toi dau roi?")
    assert with_marks.intent is without_marks.intent is Intent.ORDER_STATUS
    assert with_marks.confidence == without_marks.confidence


def test_greeting_loses_to_the_real_question(rules):
    """'chào shop, đơn hàng của em đâu' is an order question, not a greeting."""
    result = rules.classify("Chào shop, đơn hàng của em đâu rồi ạ")
    assert result.intent is Intent.ORDER_STATUS


def test_greeting_wins_when_alone(rules):
    assert rules.classify("chao shop").intent is Intent.GREETING


def test_confidence_grows_with_more_matches_but_never_reaches_one(rules):
    one_hit = rules.classify("đơn hàng")
    many_hits = rules.classify("đơn hàng của em khi nào giao, giao chưa shop, mã đơn đâu")
    assert one_hit.confidence < many_hits.confidence <= 0.95


def test_unknown_message_has_zero_confidence(rules):
    result = rules.classify("abcxyz")
    assert result.intent is Intent.OTHER
    assert result.confidence == 0.0


# ---- LLM classifier ----


@respx.mock
async def test_llm_classifier_parses_label():
    respx.post(CHAT_URL).mock(return_value=httpx.Response(200, json={"content": "shipping_fee"}))
    result = await LLMIntentClassifier(MAGIC_URL, api_key="k").classify("cho hỏi ship")
    assert result.intent is Intent.SHIPPING_FEE
    assert result.source == "llm"


@respx.mock
async def test_llm_classifier_tolerates_chatty_output():
    """Models add punctuation/extra words even when told not to."""
    respx.post(CHAT_URL).mock(return_value=httpx.Response(200, json={"content": "  order_status.\n"}))
    result = await LLMIntentClassifier(MAGIC_URL).classify("đơn đâu")
    assert result.intent is Intent.ORDER_STATUS


@respx.mock
async def test_llm_classifier_unknown_label_is_other():
    respx.post(CHAT_URL).mock(return_value=httpx.Response(200, json={"content": "khong_biet"}))
    assert (await LLMIntentClassifier(MAGIC_URL).classify("???")).intent is Intent.OTHER


@respx.mock
async def test_llm_classifier_sends_auth_and_cheap_strategy():
    route = respx.post(CHAT_URL).mock(return_value=httpx.Response(200, json={"content": "greeting"}))
    await LLMIntentClassifier(MAGIC_URL, api_key="secret").classify("hi")

    import json as jsonlib

    body = jsonlib.loads(route.calls.last.request.content)
    assert route.calls.last.request.headers["Authorization"] == "Bearer secret"
    assert body["strategy"] == "cheapest"


# ---- Hybrid ----


@respx.mock
async def test_hybrid_skips_llm_when_rules_are_confident():
    route = respx.post(CHAT_URL).mock(return_value=httpx.Response(200, json={"content": "other"}))
    hybrid = HybridIntentClassifier(llm=LLMIntentClassifier(MAGIC_URL))

    result = await hybrid.classify("đơn hàng của em khi nào giao, giao chưa shop")

    assert result.intent is Intent.ORDER_STATUS
    assert route.call_count == 0  # no spend when the rules already know


@respx.mock
async def test_hybrid_falls_back_to_llm_when_rules_are_unsure():
    respx.post(CHAT_URL).mock(return_value=httpx.Response(200, json={"content": "product_info"}))
    hybrid = HybridIntentClassifier(llm=LLMIntentClassifier(MAGIC_URL))

    result = await hybrid.classify("cái này xài sao vậy shop")

    assert result.intent is Intent.PRODUCT_INFO
    assert result.source == "llm"


@respx.mock
async def test_hybrid_keeps_rule_guess_when_llm_is_down():
    respx.post(CHAT_URL).mock(side_effect=httpx.ConnectError("down"))
    hybrid = HybridIntentClassifier(llm=LLMIntentClassifier(MAGIC_URL))

    result = await hybrid.classify("cái này xài sao vậy shop")

    assert result.source == "rules"  # degraded, not crashed


async def test_hybrid_without_llm_is_just_rules():
    result = await HybridIntentClassifier(llm=None).classify("phí ship bao nhiêu")
    assert result.intent is Intent.SHIPPING_FEE
