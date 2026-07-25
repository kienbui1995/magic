"""EcomOps end-to-end demo: tin nhắn Zalo → tra đơn trong Google Sheet → trả lời có guardrail.

Đây là deliverable Tuần 2 của roadmap: "AI trả lời được câu hỏi trạng thái đơn
hàng qua Zalo, dùng dữ liệu từ Google Sheet".

Chạy:
    python -m ecomops.example_worker

Cần 2 file config (xem connectors/*/config.example.yaml):
    connectors/zalo/config.yaml
    connectors/google_sheet/config.yaml
"""

import asyncio
import logging
import os
import sys
import threading
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT / "connectors"))

from google_sheet.connector import GoogleSheetConnector  # noqa: E402
from zalo.connector import ZaloConnector  # noqa: E402
from zalo.webhook import serve  # noqa: E402

from ecomops.intents import HybridIntentClassifier, LLMIntentClassifier  # noqa: E402
from ecomops.knowledge import MagicKnowledgeLookup  # noqa: E402
from ecomops.workflow import EcomOpsWorkflow, LLMDrafter  # noqa: E402

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("ecomops.demo")

MAGIC_URL = os.getenv("MAGIC_URL", "http://localhost:8080")
MAGIC_API_KEY = os.getenv("MAGIC_API_KEY", "")
ORDERS_RANGE = os.getenv("ECOMOPS_ORDERS_RANGE", "Orders!A1:H500")


def make_order_lookup(sheet: GoogleSheetConnector):
    """Find an order in the sheet by order code, falling back to phone."""

    async def lookup(order_code: str | None, phone: str | None) -> dict | None:
        data = await sheet.execute("read_range", {"range": ORDERS_RANGE})
        rows = data.get("values") or []
        if len(rows) < 2:
            return None

        header, *body = rows
        for row in body:
            order = sheet.map_to_common_schema({"header": header, "row": row}, "order")
            if order_code and order.get("id") == order_code:
                return order
            if phone and phone in row:
                return order
        return None

    return lookup


async def handle_message(workflow: EcomOpsWorkflow, zalo: ZaloConnector, record: dict) -> None:
    user_id = record.get("sender_id")
    text = record.get("text") or ""
    if not (user_id and text):
        return

    result = await workflow.handle(text, customer={"phone": None})
    logger.info("intent=%s action=%s trace=%s", result.intent.value, result.action, result.trace)

    await zalo.execute("send_text_message", {"user_id": user_id, "text": result.reply})

    if result.action == "handoff":
        # Nơi để nối vào Human Inbox / hệ thống case sau này.
        logger.warning(
            "HANDOFF user=%s reason=%s violations=%s",
            user_id, result.handoff_reason, [v.rule for v in result.violations],
        )


def main() -> None:
    zalo_cfg = yaml.safe_load((REPO_ROOT / "connectors/zalo/config.yaml").read_text())
    sheet_cfg = yaml.safe_load((REPO_ROOT / "connectors/google_sheet/config.yaml").read_text())

    zalo = ZaloConnector(zalo_cfg)
    sheet = GoogleSheetConnector(sheet_cfg)

    workflow = EcomOpsWorkflow(
        classifier=HybridIntentClassifier(llm=LLMIntentClassifier(MAGIC_URL, MAGIC_API_KEY)),
        drafter=LLMDrafter(MAGIC_URL, MAGIC_API_KEY),
        order_lookup=make_order_lookup(sheet),
        # Chính sách shop lấy từ MagiC Knowledge Hub — nạp trước bằng
        # `python -m ecomops.seed_knowledge`, nếu không câu hỏi về phí ship /
        # đổi trả sẽ không có dữ liệu để trả lời.
        knowledge_lookup=MagicKnowledgeLookup(MAGIC_URL, MAGIC_API_KEY),
        allowed_contacts=zalo_cfg.get("shop_hotlines", []),
    )

    # Zalo's webhook server is threaded and synchronous, so the async pipeline
    # runs on its own event loop in a background thread; each webhook thread
    # hands work to that loop instead of spinning up a fresh one per message
    # (which would also re-open the HTTP connection pools every time).
    loop = asyncio.new_event_loop()
    threading.Thread(target=loop.run_forever, daemon=True).start()

    asyncio.run_coroutine_threadsafe(zalo.connect(), loop).result()
    asyncio.run_coroutine_threadsafe(sheet.connect(), loop).result()

    def on_event(records: list[dict]) -> None:
        for record in records:
            asyncio.run_coroutine_threadsafe(handle_message(workflow, zalo, record), loop)

    port = int(zalo_cfg.get("webhook", {}).get("port", 9100))
    logger.info("EcomOps demo listening for Zalo webhooks on :%d", port)
    serve(zalo, on_event, port=port)


if __name__ == "__main__":
    main()
