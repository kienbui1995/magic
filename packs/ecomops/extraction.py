"""Pull the order code / phone number out of a free-text customer message.

Cheap regex extraction on purpose: an order lookup only needs the identifier,
and asking an LLM to copy a number out of a sentence is slow, costs money, and
occasionally hallucinates a digit.
"""

import re
from dataclasses import dataclass

# Vietnamese mobile numbers: 0XXXXXXXXX (10 digits) or +84/84 + 9 digits.
PHONE_PATTERN = re.compile(r"(?:(?<![\d+])(?:\+?84|0))(\d{9})(?!\d)")

# Ordered most- to least-specific: an explicit "#..." or a "DH..." prefix is a
# far stronger signal than a bare run of digits.
ORDER_CODE_PATTERNS = [
    re.compile(r"#\s*([A-Za-z0-9][A-Za-z0-9\-_]{2,19})"),
    re.compile(r"\b((?:DH|MDH|MA|ORD|HD|SO)[-_]?\d{3,15})\b", re.IGNORECASE),
    re.compile(r"(?<!\d)(\d{6,15})(?!\d)"),
]


@dataclass
class ExtractedInfo:
    order_code: str | None = None
    phone: str | None = None


def canonical_phone(value: str) -> str | None:
    """Fold any spelling of a Vietnamese mobile number to local 0XXXXXXXXX form.

    Handles "+84 987 654 321", "84987654321", "0987-654-321" and "987654321".
    Returns None if it isn't a plausible VN mobile number, so callers can tell
    "not a phone" apart from "a phone we normalized".
    """
    digits = re.sub(r"\D", "", value)
    if digits.startswith("84"):
        digits = "0" + digits[2:]
    elif len(digits) == 9:
        digits = "0" + digits
    return digits if len(digits) == 10 and digits.startswith("0") else None


def extract_phone(message: str) -> str | None:
    """Return the phone in local 0XXXXXXXXX form, whatever prefix was typed."""
    match = PHONE_PATTERN.search(message)
    return f"0{match.group(1)}" if match else None


def extract_order_code(message: str, known_phone: str | None = None) -> str | None:
    """Find an order code, ignoring digits that are actually the phone number.

    Without this, "đơn của em sđt 0901234567" would happily return the phone as
    an order code via the bare-digits pattern and the lookup would miss.
    """
    haystack = message
    phone = known_phone if known_phone is not None else extract_phone(message)
    if phone:
        # Blank out every spelling of that phone (0..., 84..., +84...) so the
        # numeric fallback pattern can't pick it up.
        local = phone.lstrip("0")
        haystack = re.sub(rf"(?:\+?84|0)?{re.escape(local)}", " ", haystack)

    for pattern in ORDER_CODE_PATTERNS:
        match = pattern.search(haystack)
        if match:
            return match.group(1)
    return None


def extract(message: str) -> ExtractedInfo:
    phone = extract_phone(message)
    return ExtractedInfo(order_code=extract_order_code(message, known_phone=phone), phone=phone)
