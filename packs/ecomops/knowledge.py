"""Knowledge lookup for EcomOps drafts.

The system prompt tells the model to answer *only* from the "Dữ liệu" block, so
without a knowledge source the shipping-fee and return-policy intents have
nothing to answer from — the bot can only say it will check and get back. This
module supplies that block.

Two backends:

- `MagicKnowledgeLookup` — reads MagiC's Knowledge Hub (`GET /api/v1/knowledge`),
  so policies live in one place shared with every other MagiC workload.
- `StaticKnowledgeLookup` — a YAML file, for shops running the pack without a
  Knowledge Hub (in-memory core, no Postgres).

Both are plain async callables matching the workflow's `knowledge_lookup`
signature, so they're interchangeable.
"""

import logging
from pathlib import Path
from typing import Any

import httpx

from .intents import Intent

logger = logging.getLogger("magic_packs.ecomops.knowledge")

# Tag every EcomOps entry carries, so the pack's lookups don't drag in unrelated
# org knowledge that happens to share a keyword.
ECOMOPS_TAG = "ecomops"

# Per-intent tag + the keyword query sent to the Hub. The Hub's `q` search is
# keyword-based over title/content, so the query is deliberately in the same
# Vietnamese wording the sample entries use.
INTENT_TOPICS: dict[Intent, tuple[str, str]] = {
    Intent.SHIPPING_FEE: ("shipping", "phí ship vận chuyển"),
    Intent.RETURN_EXCHANGE: ("return", "đổi trả hoàn tiền bảo hành"),
    Intent.ORDER_STATUS: ("order", "trạng thái đơn hàng giao hàng"),
    Intent.PRODUCT_INFO: ("product", "sản phẩm size màu chất liệu"),
    Intent.COMPLAINT: ("complaint", "khiếu nại xử lý sự cố"),
}


def topic_for(intent: Intent) -> str | None:
    """The knowledge tag an intent needs, or None if it needs no policy data."""
    topic = INTENT_TOPICS.get(intent)
    return topic[0] if topic else None


class MagicKnowledgeLookup:
    """Pulls shop policy snippets from MagiC's Knowledge Hub."""

    def __init__(self, magic_url: str, api_key: str = "", top_k: int = 3, timeout: float = 10.0):
        self.magic_url = magic_url.rstrip("/")
        self.api_key = api_key
        self.top_k = top_k
        self.timeout = timeout

    async def __call__(self, intent: Intent) -> list[str]:
        topic = INTENT_TOPICS.get(intent)
        if topic is None:
            return []
        tag, query = topic

        headers = {"Authorization": f"Bearer {self.api_key}"} if self.api_key else {}
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.get(
                    f"{self.magic_url}/api/v1/knowledge", headers=headers, params={"q": query}
                )
                resp.raise_for_status()
                entries = resp.json()
        except (httpx.HTTPError, ValueError) as e:
            # Missing policy data degrades the answer; it shouldn't drop the
            # message. The workflow still drafts, and the guardrails still stop
            # anything the model invents to fill the gap.
            logger.warning("knowledge lookup failed for %s: %s", intent.value, e)
            return []

        if not isinstance(entries, list):
            return []

        # The Hub has no server-side tag filter, so narrow here: an entry must
        # carry both the ecomops tag and this intent's topic tag.
        snippets = []
        for entry in entries:
            if not isinstance(entry, dict):
                continue
            tags = entry.get("tags") or []
            if ECOMOPS_TAG in tags and tag in tags:
                content = (entry.get("content") or "").strip()
                if content:
                    snippets.append(content)
        return snippets[: self.top_k]


class StaticKnowledgeLookup:
    """Serves policy snippets from a local mapping — no Knowledge Hub needed."""

    def __init__(self, entries: dict[str, list[str]] | None = None):
        self.entries = entries or {}

    async def __call__(self, intent: Intent) -> list[str]:
        tag = topic_for(intent)
        return list(self.entries.get(tag, [])) if tag else []

    @classmethod
    def from_yaml(cls, path: str | Path) -> "StaticKnowledgeLookup":
        """Load from a YAML mapping of topic -> list of snippets.

        Imports yaml lazily so the rest of the pack stays dependency-free for
        callers that build their knowledge mapping in code.
        """
        import yaml

        data = yaml.safe_load(Path(path).read_text(encoding="utf-8")) or {}
        return cls({str(k): [str(s) for s in (v or [])] for k, v in data.items()})


def sample_knowledge_path() -> Path:
    return Path(__file__).parent / "sample_knowledge.yaml"


def knowledge_entries_for_seeding(path: str | Path | None = None) -> list[dict[str, Any]]:
    """Turn the sample YAML into `POST /api/v1/knowledge` payloads.

    Each entry is tagged `ecomops` plus its topic so `MagicKnowledgeLookup` can
    find it again.
    """
    import yaml

    src = Path(path) if path else sample_knowledge_path()
    data = yaml.safe_load(src.read_text(encoding="utf-8")) or {}

    payloads = []
    for topic, snippets in data.items():
        for i, snippet in enumerate(snippets or [], start=1):
            payloads.append(
                {
                    "title": f"EcomOps — {topic} #{i}",
                    "content": str(snippet).strip(),
                    "tags": [ECOMOPS_TAG, str(topic)],
                    "scope": "org",
                }
            )
    return payloads


async def seed_knowledge_hub(
    magic_url: str, api_key: str = "", path: str | Path | None = None, timeout: float = 15.0
) -> int:
    """Push the sample knowledge base into MagiC's Knowledge Hub. Returns count."""
    payloads = knowledge_entries_for_seeding(path)
    headers = {"Authorization": f"Bearer {api_key}"} if api_key else {}

    async with httpx.AsyncClient(timeout=timeout) as client:
        for payload in payloads:
            resp = await client.post(
                f"{magic_url.rstrip('/')}/api/v1/knowledge", headers=headers, json=payload
            )
            resp.raise_for_status()
    return len(payloads)
