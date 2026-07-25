# MagiC Solution Packs

Pack = workflow nghiệp vụ cho một ngành cụ thể, xây trên Connector Framework và
core MagiC.

Khác biệt với `connectors/`:

- **Connector** trả lời câu hỏi *"kết nối tới nền tảng nào"* (Zalo, Google Sheet,
  Nhanh.vn) — chỉ gọi API và map dữ liệu về Common Schema.
- **Pack** trả lời câu hỏi *"làm nghiệp vụ gì"* (trả lời khách, tra đơn, kiểm tra
  guardrail) — chứa logic nghiệp vụ, và **không** được hard-code nền tảng nào.

```
packs/
└── ecomops/     # Chăm sóc khách hàng cho shop bán hàng online
```

## Nguyên tắc

1. Pack nhận mọi phụ thuộc bên ngoài qua **dependency injection** (classifier,
   tra đơn, model soạn tin), không tự gọi connector cụ thể — nhờ vậy cùng một
   workflow chạy được trên Zalo, Facebook, hay test harness.
2. Dữ liệu ra/vào dùng **Common Data Schema** (`sdk/python/magic_ai_sdk/connectors/schema.py`).
3. Mọi câu trả lời gửi tới khách phải đi qua **guardrail** trước.
4. Gọi LLM qua **LLM gateway của MagiC** (`POST /api/v1/llm/chat`) thay vì SDK của
   nhà cung cấp, để chi phí token được Cost Controller ghi nhận cùng mọi workload khác.
