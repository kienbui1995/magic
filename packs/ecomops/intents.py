"""Intent classification for EcomOps customer messages (Vietnamese).

Two classifiers, meant to be layered:

- `RuleBasedIntentClassifier` — keyword matching over diacritics-normalized text.
  Free, instant, and handles the bulk of real shop traffic, which is short and
  formulaic ("đơn hàng của em tới đâu rồi ạ").
- `LLMIntentClassifier` — asks MagiC's own LLM gateway (`POST /api/v1/llm/chat`),
  so the pack stays provider-agnostic and every call is cost-tracked by core.

`HybridIntentClassifier` runs the rules first and only pays for an LLM call when
the rules are unsure — quality where it matters, no spend where it doesn't.
"""

import re
from dataclasses import dataclass, field
from enum import Enum

import httpx

from .text import normalize


class Intent(str, Enum):
    ORDER_STATUS = "order_status"
    SHIPPING_FEE = "shipping_fee"
    RETURN_EXCHANGE = "return_exchange"
    COMPLAINT = "complaint"
    PRODUCT_INFO = "product_info"
    GREETING = "greeting"
    OTHER = "other"


@dataclass
class IntentResult:
    intent: Intent
    confidence: float
    matched: list[str] = field(default_factory=list)
    source: str = "rules"


# Keywords are written in normalized form (lowercase, no diacritics) because the
# incoming message is normalized the same way — that makes each entry match both
# "đơn hàng" and "don hang" without duplicating the list.
#
# Deliberately multi-word where a single word would collide: bare "hong" would
# match "màu hồng" (pink) as a damage complaint, "vo" would match "vợ"/"vô", and
# "bao nhieu" alone spans both price and shipping questions.
INTENT_KEYWORDS: dict[Intent, list[str]] = {
    Intent.ORDER_STATUS: [
        "don hang", "kien hang", "van don", "ma don", "tinh trang don", "check don",
        "tra cuu don", "khi nao nhan", "khi nao giao", "bao gio nhan", "bao gio giao",
        "giao chua", "den chua", "toi dau", "dang o dau", "shipper", "da gui chua",
    ],
    Intent.SHIPPING_FEE: [
        "phi ship", "phi van chuyen", "phi giao hang", "tien ship", "cuoc van chuyen",
        "ship bao nhieu", "ship het bao nhieu", "freeship", "mien phi ship", "mien ship",
    ],
    Intent.RETURN_EXCHANGE: [
        "doi tra", "doi hang", "tra hang", "tra lai", "hoan tra", "doi size", "doi mau",
        "chinh sach doi", "bao hanh", "hoan tien",
    ],
    Intent.COMPLAINT: [
        "khieu nai", "phan anh", "that vong", "te qua", "lua dao", "boc phot",
        "kem chat luong", "hang loi", "bi hong", "bi vo", "bi rach", "loi san pham",
        "giao sai", "giao thieu", "thieu hang", "khong dung mau", "khong dung size",
        "buc xuc", "cham qua", "qua lau",
    ],
    Intent.PRODUCT_INFO: [
        "con hang", "con size", "con mau", "co san", "gia bao nhieu", "gia the nao",
        "bao nhieu tien", "chat lieu", "thong tin san pham", "size nao", "mau nao",
    ],
    Intent.GREETING: [
        "xin chao", "chao shop", "chao ban", "chao em", "alo", "hello", "hi shop",
    ],
}


# Vietnamese slots pronouns and particles inside otherwise fixed phrases —
# "khi nào **em** nhận được hàng" should still match the keyword "khi nao nhan".
# So keyword tokens are matched in order with up to this many filler words
# between them, rather than as one literal substring. Kept at 1: it absorbs the
# common single-pronoun insertion without letting a keyword match words scattered
# across a whole sentence.
_MAX_GAP_WORDS = 1


def _compile_keyword(keyword: str) -> re.Pattern[str]:
    gap = rf"(?:\s+\S+){{0,{_MAX_GAP_WORDS}}}\s+"
    body = gap.join(re.escape(token) for token in keyword.split())
    return re.compile(rf"(?<!\w){body}")


class RuleBasedIntentClassifier:
    """Keyword classifier over diacritics-normalized Vietnamese text."""

    def __init__(self, keywords: dict[Intent, list[str]] | None = None):
        self.keywords = keywords if keywords is not None else INTENT_KEYWORDS
        self._compiled = {
            intent: [(kw, _compile_keyword(kw)) for kw in kws]
            for intent, kws in self.keywords.items()
        }

    def classify(self, message: str) -> IntentResult:
        norm = normalize(message)

        hits: dict[Intent, list[str]] = {}
        for intent, compiled in self._compiled.items():
            found = [kw for kw, pattern in compiled if pattern.search(norm)]
            if found:
                hits[intent] = found

        # A greeting is almost always a prefix to the real question ("chào shop,
        # đơn hàng của em đâu rồi") — it should only win when it's all there is.
        if len(hits) > 1:
            hits.pop(Intent.GREETING, None)

        if not hits:
            return IntentResult(Intent.OTHER, 0.0, [])

        # Rank by total matched length, so a specific phrase outranks a short one.
        best = max(hits, key=lambda i: (sum(len(kw) for kw in hits[i]), len(hits[i])))
        return IntentResult(best, _confidence(len(hits[best])), sorted(hits[best]))


def _confidence(n_hits: int) -> float:
    """Heuristic: one keyword is a decent signal, several is a strong one.

    Capped below 1.0 — keyword matching should never claim certainty, so a
    hybrid/LLM layer above can still override it if it wants to.
    """
    return min(0.95, 0.5 + 0.15 * n_hits)


_LLM_SYSTEM_PROMPT = """Bạn là bộ phân loại ý định cho chatbot chăm sóc khách hàng của shop bán hàng online Việt Nam.
Phân loại tin nhắn của khách vào ĐÚNG MỘT nhãn trong danh sách sau:

- order_status: hỏi tình trạng/vị trí đơn hàng, khi nào nhận được hàng
- shipping_fee: hỏi phí ship, phí vận chuyển
- return_exchange: muốn đổi hàng, trả hàng, bảo hành, hoàn tiền
- complaint: khiếu nại, phàn nàn về hàng lỗi/giao sai/giao chậm/thái độ
- product_info: hỏi thông tin sản phẩm, giá, size, màu, còn hàng không
- greeting: chỉ chào hỏi, chưa có yêu cầu cụ thể
- other: không thuộc các nhóm trên

Chỉ trả lời bằng đúng một nhãn, viết thường, không giải thích, không thêm dấu câu."""

_VALID_LABELS = {i.value for i in Intent}


class LLMIntentClassifier:
    """Classifies via MagiC's LLM gateway (`POST /api/v1/llm/chat`).

    Uses the `cheapest` routing strategy — labelling a short message doesn't
    need a frontier model, and this runs on every inbound customer message.
    """

    def __init__(
        self,
        magic_url: str,
        api_key: str = "",
        model: str = "",
        timeout: float = 15.0,
        strategy: str = "cheapest",
    ):
        self.magic_url = magic_url.rstrip("/")
        self.api_key = api_key
        self.model = model
        self.timeout = timeout
        self.strategy = strategy

    async def classify(self, message: str) -> IntentResult:
        headers = {"Authorization": f"Bearer {self.api_key}"} if self.api_key else {}
        payload: dict = {
            "messages": [
                {"role": "system", "content": _LLM_SYSTEM_PROMPT},
                {"role": "user", "content": message},
            ],
            "strategy": self.strategy,
            "max_tokens": 16,
        }
        if self.model:
            payload["model"] = self.model

        async with httpx.AsyncClient(timeout=self.timeout) as client:
            resp = await client.post(f"{self.magic_url}/api/v1/llm/chat", headers=headers, json=payload)
            resp.raise_for_status()
            content = (resp.json().get("content") or "").strip().lower()

        # Models like to add punctuation or a stray word even when told not to;
        # take the first token that is a label we actually know.
        for token in content.replace(",", " ").replace(".", " ").split():
            if token in _VALID_LABELS:
                return IntentResult(Intent(token), 0.9, [], source="llm")
        return IntentResult(Intent.OTHER, 0.0, [], source="llm")


class HybridIntentClassifier:
    """Rules first; fall back to the LLM only when the rules aren't confident."""

    def __init__(self, llm: LLMIntentClassifier | None = None, threshold: float = 0.65):
        self.rules = RuleBasedIntentClassifier()
        self.llm = llm
        self.threshold = threshold

    async def classify(self, message: str) -> IntentResult:
        result = self.rules.classify(message)
        if result.confidence >= self.threshold or self.llm is None:
            return result
        try:
            llm_result = await self.llm.classify(message)
        except (httpx.HTTPError, ValueError):
            # LLM unavailable is not a reason to drop the message — keep the
            # rule-based guess and let the workflow's low-confidence handoff
            # rule decide whether a human should look at it.
            return result
        return llm_result if llm_result.intent != Intent.OTHER else result
