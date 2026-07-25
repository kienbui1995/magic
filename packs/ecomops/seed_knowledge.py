"""Nạp Knowledge Base mẫu vào MagiC Knowledge Hub.

    cd packs
    MAGIC_URL=http://localhost:8080 MAGIC_API_KEY=your-key python -m ecomops.seed_knowledge

Truyền đường dẫn file YAML của shop để nạp chính sách thật thay cho dữ liệu mẫu:

    python -m ecomops.seed_knowledge /duong/dan/chinh-sach-shop.yaml
"""

import asyncio
import logging
import os
import sys

from .knowledge import knowledge_entries_for_seeding, sample_knowledge_path, seed_knowledge_hub

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("ecomops.seed")


def main() -> None:
    magic_url = os.getenv("MAGIC_URL", "http://localhost:8080")
    api_key = os.getenv("MAGIC_API_KEY", "")
    path = sys.argv[1] if len(sys.argv) > 1 else sample_knowledge_path()

    try:
        entries = knowledge_entries_for_seeding(path)
    except (ValueError, FileNotFoundError) as e:
        logger.error("%s", e)
        sys.exit(2)

    if path == sample_knowledge_path():
        logger.warning(
            "Đang nạp DỮ LIỆU MẪU từ %s — thay bằng chính sách thật của shop trước khi dùng cho khách.",
            path,
        )

    logger.info("Nạp %d entry vào %s ...", len(entries), magic_url)
    count = asyncio.run(seed_knowledge_hub(magic_url, api_key, path))
    logger.info("Xong: đã nạp %d entry.", count)


if __name__ == "__main__":
    main()
