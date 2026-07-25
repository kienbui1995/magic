# EcomOps Pack

Workflow chăm sóc khách hàng cho shop bán hàng online Việt Nam:

```
Nhận tin → Phân loại ý định → Trích mã đơn/SĐT → Tra đơn hàng
        → Soạn câu trả lời → Kiểm tra Guardrail → Gửi hoặc chuyển nhân viên
```

Pack này chạy như **worker độc lập**, không nằm trong core Go. Mọi phụ thuộc bên
ngoài (classifier, tra đơn, model soạn tin) đều được **inject vào**, nên workflow
tự nó không gọi mạng và không biết gì về Zalo hay Google Sheet — cùng một workflow
chạy được trên bất kỳ kênh nào.

## 1. Cài đặt

Chạy từ thư mục gốc repo (`magic/`):

```bash
pip install -e 'sdk/python[connectors]'
pip install -r packs/ecomops/requirements.txt
pytest packs/ecomops/tests
```

## 2. Các thành phần

| Module | Vai trò |
|--------|---------|
| `text.py` | Chuẩn hoá tiếng Việt (bỏ dấu) để so khớp không phụ thuộc cách gõ |
| `intents.py` | Phân loại ý định: `RuleBasedIntentClassifier`, `LLMIntentClassifier`, `HybridIntentClassifier` |
| `extraction.py` | Trích mã đơn hàng & số điện thoại từ tin nhắn |
| `prompts.py` | Thư viện prompt tiếng Việt theo từng ý định |
| `guardrails.py` | Chặn câu trả lời rủi ro trước khi gửi cho khách |
| `workflow.py` | `EcomOpsWorkflow` — pipeline nối tất cả lại |

### Ý định được hỗ trợ

`order_status` (tình trạng đơn) · `shipping_fee` (phí ship) · `return_exchange`
(đổi trả) · `complaint` (khiếu nại) · `product_info` (thông tin sản phẩm) ·
`greeting` · `other`

Classifier **không phụ thuộc dấu tiếng Việt** — "đơn hàng của em đâu rồi" và
"don hang cua em dau roi" cho cùng kết quả, vì khách hay gõ không dấu. Nó cũng
cho phép chèn 1 từ đệm giữa các từ khoá, nên "khi nào **em** nhận được hàng" vẫn
khớp từ khoá "khi nao nhan".

`HybridIntentClassifier` chạy rule trước, **chỉ gọi LLM khi rule không chắc** —
tiết kiệm chi phí vì phần lớn tin nhắn của khách rất ngắn và lặp lại.

### Guardrail (quan trọng nhất)

Câu trả lời do LLM soạn **không mặc nhiên an toàn** để gửi từ tài khoản chính
thức của shop. Các rule mặc định chặn:

| Rule | Chặn điều gì |
|------|--------------|
| `empty_draft` | Câu trả lời rỗng |
| `leaked_contact` | SĐT/email không phải của khách hoặc của shop (lộ thông tin khách khác) |
| `delivery_promise` | Hứa chắc chắn ngày giao ("chắc chắn giao ngày mai") |
| `compensation_promise` | Tự hứa hoàn tiền/voucher/giảm giá khi chưa được duyệt |
| `internal_leak` | Lộ system prompt, API key, ghi chú nội bộ |

Guardrail cũng chạy trên text đã bỏ dấu, nên **không thể lách bằng cách gõ không dấu**.

Bất kỳ vi phạm mức `BLOCK` nào → workflow **không gửi draft đó**, mà trả về câu
trả lời an toàn và đánh dấu `action="handoff"` để nhân viên tiếp nhận.

## 3. Dùng nhanh

```python
import asyncio
from ecomops import EcomOpsWorkflow, RuleBasedIntentClassifier

async def my_order_lookup(order_code, phone):
    return {"id": order_code, "status": "shipping", "total_amount": 250000}

async def my_drafter(messages):
    ...  # gọi LLM, hoặc dùng ecomops.workflow.LLMDrafter

async def main():
    wf = EcomOpsWorkflow(
        classifier=RuleBasedIntentClassifier(),
        drafter=my_drafter,
        order_lookup=my_order_lookup,
    )
    result = await wf.handle("đơn #DH12345 của em tới đâu rồi ạ")
    print(result.action, result.intent, result.reply)

asyncio.run(main())
```

`WorkflowResult`:

- `action`: `"send"` (bot xử lý xong) hoặc `"handoff"` (cần nhân viên tiếp nhận)
- `reply`: **luôn an toàn để gửi** — nếu draft bị guardrail chặn, đây là câu trả lời dự phòng
- `intent`, `confidence`, `extracted`, `order`, `violations`, `handoff_reason`, `trace`

Khi nào chuyển nhân viên: khiếu nại (`complaint`), độ tin cậy thấp, guardrail chặn,
hoặc soạn tin lỗi.

## 4. Demo end-to-end (Zalo + Google Sheet)

`example_worker.py` nối: webhook Zalo → tra đơn trong Google Sheet → trả lời có
guardrail → gửi lại qua Zalo.

```bash
# cần connectors/zalo/config.yaml và connectors/google_sheet/config.yaml
export MAGIC_URL=http://localhost:8080
export MAGIC_API_KEY=your-key
cd packs && python -m ecomops.example_worker
```

Sheet cần tab `Orders` với dòng header khớp Common Schema
(`id`, `customer_id`, `status`, `total_amount`, `shipping_fee`, `created_at`) —
xem `connectors/google_sheet/README.md`.

## 5. Giới hạn hiện tại

- Từ khoá phân loại là **danh sách cố định trong code**, chưa cấu hình được qua file.
- Chưa có Case Management (tạo case khi khiếu nại/đổi trả) — thuộc Tuần 3–4 của roadmap.
- `example_worker.py` quét toàn bộ dải `Orders!A1:H500` mỗi lần tra đơn; với sheet
  lớn nên thay bằng Nhanh.vn connector hoặc thêm cache.
- Guardrail dựa trên cụm từ, không phải mô hình — bắt được các mẫu hứa hẹn phổ biến,
  nhưng không thay thế được người duyệt với các đơn giá trị cao.
