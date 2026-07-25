"""Order-code / phone extraction tests."""

import pytest

from ecomops.extraction import extract, extract_order_code, extract_phone


@pytest.mark.parametrize(
    "message,expected",
    [
        ("sdt cua em la 0901234567", "0901234567"),
        ("lien he +84901234567 nhe", "0901234567"),
        ("so 84901234567", "0901234567"),
        ("khong co so nao", None),
    ],
)
def test_extract_phone(message, expected):
    assert extract_phone(message) == expected


@pytest.mark.parametrize(
    "message,expected",
    [
        ("cho hoi don #DH12345", "DH12345"),
        ("ma don DH-556677 giao chua", "DH-556677"),
        ("don hang ORD_998877", "ORD_998877"),
        ("ma 987654321 toi dau", "987654321"),
        ("khong co ma nao ca", None),
    ],
)
def test_extract_order_code(message, expected):
    assert extract_order_code(message) == expected


def test_phone_is_not_mistaken_for_an_order_code():
    """A bare 10-digit phone must not be returned as the order id, or the
    order lookup silently misses."""
    assert extract_order_code("don cua em sdt 0901234567 giao chua") is None


def test_phone_and_order_code_coexist():
    info = extract("em dat hang, sdt 0901234567, ma don DH-556677")
    assert info.phone == "0901234567"
    assert info.order_code == "DH-556677"


def test_international_phone_does_not_leak_into_order_code():
    info = extract("sdt +84901234567 kiem tra giup em")
    assert info.phone == "0901234567"
    assert info.order_code is None


def test_prefixed_code_wins_over_bare_digits():
    """'#'/'DH' prefixes are a much stronger signal than a loose number."""
    assert extract_order_code("don #DH12345 ngay 20260721") == "DH12345"


def test_empty_message():
    info = extract("")
    assert info.order_code is None and info.phone is None
