"""Knowledge lookup tests — Hub calls mocked via respx, no network."""

import httpx
import pytest
import respx

from ecomops.intents import Intent
from ecomops.knowledge import (
    ECOMOPS_TAG,
    MagicKnowledgeLookup,
    StaticKnowledgeLookup,
    knowledge_entries_for_seeding,
    sample_knowledge_path,
    seed_knowledge_hub,
    topic_for,
)

MAGIC_URL = "http://magic:8080"
KB_URL = f"{MAGIC_URL}/api/v1/knowledge"


def entry(content, tags):
    return {"id": "k1", "title": "t", "content": content, "tags": tags}


# ---- topic mapping ----


@pytest.mark.parametrize(
    "intent,expected",
    [
        (Intent.SHIPPING_FEE, "shipping"),
        (Intent.RETURN_EXCHANGE, "return"),
        (Intent.ORDER_STATUS, "order"),
        (Intent.PRODUCT_INFO, "product"),
        (Intent.COMPLAINT, "complaint"),
        (Intent.GREETING, None),
        (Intent.OTHER, None),
    ],
)
def test_topic_for(intent, expected):
    assert topic_for(intent) == expected


# ---- MagicKnowledgeLookup ----


@respx.mock
async def test_returns_matching_entries():
    respx.get(KB_URL).mock(
        return_value=httpx.Response(200, json=[entry("Phí ship nội thành 25.000đ", [ECOMOPS_TAG, "shipping"])])
    )
    snippets = await MagicKnowledgeLookup(MAGIC_URL)(Intent.SHIPPING_FEE)
    assert snippets == ["Phí ship nội thành 25.000đ"]


@respx.mock
async def test_filters_out_other_topics():
    """A keyword hit on the return policy must not answer a shipping question."""
    respx.get(KB_URL).mock(
        return_value=httpx.Response(
            200,
            json=[
                entry("Đổi trả trong 7 ngày", [ECOMOPS_TAG, "return"]),
                entry("Phí ship 25.000đ", [ECOMOPS_TAG, "shipping"]),
            ],
        )
    )
    assert await MagicKnowledgeLookup(MAGIC_URL)(Intent.SHIPPING_FEE) == ["Phí ship 25.000đ"]


@respx.mock
async def test_filters_out_non_ecomops_org_knowledge():
    """Unrelated org knowledge that happens to match the keyword is excluded."""
    respx.get(KB_URL).mock(
        return_value=httpx.Response(200, json=[entry("Nội quy công ty về phí ship nội bộ", ["hr", "shipping"])])
    )
    assert await MagicKnowledgeLookup(MAGIC_URL)(Intent.SHIPPING_FEE) == []


@respx.mock
async def test_respects_top_k():
    respx.get(KB_URL).mock(
        return_value=httpx.Response(200, json=[entry(f"snippet {i}", [ECOMOPS_TAG, "shipping"]) for i in range(5)])
    )
    assert len(await MagicKnowledgeLookup(MAGIC_URL, top_k=2)(Intent.SHIPPING_FEE)) == 2


async def test_intent_without_a_topic_skips_the_call():
    """Greetings need no policy data — don't spend a request on them."""
    with respx.mock:
        route = respx.get(KB_URL).mock(return_value=httpx.Response(200, json=[]))
        assert await MagicKnowledgeLookup(MAGIC_URL)(Intent.GREETING) == []
        assert route.call_count == 0


@respx.mock
async def test_hub_error_degrades_to_no_knowledge():
    respx.get(KB_URL).mock(side_effect=httpx.ConnectError("down"))
    assert await MagicKnowledgeLookup(MAGIC_URL)(Intent.SHIPPING_FEE) == []


@respx.mock
async def test_unexpected_payload_shape_is_ignored():
    respx.get(KB_URL).mock(return_value=httpx.Response(200, json={"error": "nope"}))
    assert await MagicKnowledgeLookup(MAGIC_URL)(Intent.SHIPPING_FEE) == []


@respx.mock
async def test_sends_auth_header_and_query():
    route = respx.get(KB_URL).mock(return_value=httpx.Response(200, json=[]))
    await MagicKnowledgeLookup(MAGIC_URL, api_key="secret")(Intent.RETURN_EXCHANGE)

    assert route.calls.last.request.headers["Authorization"] == "Bearer secret"
    assert "đổi trả" in route.calls.last.request.url.params["q"]


# ---- StaticKnowledgeLookup ----


async def test_static_lookup_by_topic():
    lookup = StaticKnowledgeLookup({"shipping": ["Phí ship 25k"], "return": ["Đổi trả 7 ngày"]})
    assert await lookup(Intent.SHIPPING_FEE) == ["Phí ship 25k"]
    assert await lookup(Intent.RETURN_EXCHANGE) == ["Đổi trả 7 ngày"]
    assert await lookup(Intent.GREETING) == []


async def test_static_lookup_missing_topic_is_empty():
    assert await StaticKnowledgeLookup({})(Intent.SHIPPING_FEE) == []


async def test_static_lookup_from_sample_yaml():
    lookup = StaticKnowledgeLookup.from_yaml(sample_knowledge_path())
    shipping = await lookup(Intent.SHIPPING_FEE)
    assert shipping and any("phí ship" in s.lower() for s in shipping)


# ---- seeding ----


def test_sample_yaml_produces_tagged_entries():
    payloads = knowledge_entries_for_seeding()
    assert payloads
    for p in payloads:
        assert ECOMOPS_TAG in p["tags"]
        assert len(p["tags"]) == 2  # ecomops + topic
        assert p["content"].strip()
        assert p["scope"] == "org"


def test_every_sample_topic_is_reachable_from_an_intent():
    """A topic in the YAML that no intent maps to would be dead data."""
    topics = {p["tags"][1] for p in knowledge_entries_for_seeding()}
    reachable = {topic_for(i) for i in Intent} - {None}
    assert topics <= reachable, f"unreachable topics in sample_knowledge.yaml: {topics - reachable}"


@respx.mock
async def test_seed_posts_every_entry():
    route = respx.post(KB_URL).mock(return_value=httpx.Response(200, json={"id": "k1"}))
    count = await seed_knowledge_hub(MAGIC_URL, api_key="k")
    assert count == len(knowledge_entries_for_seeding())
    assert route.call_count == count
    assert route.calls.last.request.headers["Authorization"] == "Bearer k"
