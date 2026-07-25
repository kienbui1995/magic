# MagiC Connectors

Nơi chứa các connector kết nối MagiC với các nền tảng bên ngoài (Zalo, Google Sheet, Nhanh.vn, Base.vn...).

Mỗi connector implement `BaseConnector` từ Connector Framework (`magic_ai_sdk.connectors`, xem
[`sdk/python/magic_ai_sdk/connectors/`](../sdk/python/magic_ai_sdk/connectors/)) và chạy như một
**worker độc lập** — connector không nằm trong core Go, core chỉ điều phối task. Điều này giữ
core sạch và không phụ thuộc vào bất kỳ nền tảng cụ thể nào.

## Cấu trúc

```
connectors/
├── zalo/            # Zalo OA Connector — nhận/gửi tin nhắn qua Zalo Official Account
├── google_sheet/    # Google Sheet Connector — đọc/ghi đơn hàng, khách hàng
├── nhanh/           # (sắp có) tích hợp Nhanh.vn
└── base/            # (sắp có) tích hợp Base.vn
```

Đây là **monorepo giai đoạn đầu** — mỗi thư mục con có thể tách thành repo riêng
(`magic-connectors`) khi cần, không ảnh hưởng code bên trong.

## Viết connector mới

1. Tạo thư mục mới trong `connectors/<ten-nen-tang>/`.
2. Implement `BaseConnector` (xem `connectors/zalo/connector.py` làm ví dụ):
   `connect`, `disconnect`, `execute`, `map_to_common_schema`, `map_from_common_schema`.
3. Mọi dữ liệu ra/vào phải map về **Common Data Schema**
   (`Customer`, `Order`, `Case`, `Conversation`, `Message`, `KnowledgeDocument` —
   định nghĩa tại `sdk/python/magic_ai_sdk/connectors/schema.py`).
4. Không hard-code logic nghiệp vụ (intent classify, guardrail...) vào connector —
   logic đó thuộc về workflow gọi connector.
5. Đăng ký connector qua `ConnectorRegistry.register(name, YourConnectorClass)`.

## Cài đặt để phát triển

```bash
pip install -e sdk/python[connectors,dev]
pip install -r connectors/zalo/requirements.txt
pip install -r connectors/google_sheet/requirements.txt
pytest connectors/zalo/tests connectors/google_sheet/tests
```
