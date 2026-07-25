"""Vietnamese text normalization shared by the intent and guardrail engines.

Vietnamese customers very often type without diacritics ("don hang cua toi
dau roi" instead of "đơn hàng của tôi đâu rồi"). Normalizing both the incoming
message and our keyword lists to a diacritics-free lowercase form lets a single
keyword list match either spelling, instead of maintaining two of everything.
"""

import unicodedata


def strip_diacritics(text: str) -> str:
    """Remove Vietnamese tone/vowel marks: 'đơn hàng' -> 'don hang'."""
    # đ/Đ are single codepoints (U+0111/U+0110) that do NOT decompose under NFD,
    # so the combining-mark filter below can't reach them — map them by hand first.
    text = text.replace("đ", "d").replace("Đ", "D")
    decomposed = unicodedata.normalize("NFD", text)
    return "".join(ch for ch in decomposed if unicodedata.category(ch) != "Mn")


def normalize(text: str) -> str:
    """Lowercase + diacritics-free + whitespace-collapsed form used for matching."""
    return " ".join(strip_diacritics(text).lower().split())
