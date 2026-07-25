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
| `knowledge.py` | Lấy chính sách shop (phí ship, đổi trả…) cho model soạn tin |
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

## 4. Knowledge Base (bắt buộc nếu muốn trả lời phí ship / đổi trả)

System prompt bắt model **chỉ được trả lời dựa trên phần "Dữ liệu"**. Nếu không
cấp Knowledge Base thì câu hỏi về phí ship hay chính sách đổi trả sẽ không có gì
để trả lời — bot chỉ có thể nói sẽ kiểm tra lại. Đây là chủ ý: thà nói "để shop
kiểm tra" còn hơn bịa ra một con số.

Hai cách cấp dữ liệu:

**a) MagiC Knowledge Hub** (khuyến nghị — dùng chung với các workload khác):

```bash
cd packs
MAGIC_URL=http://localhost:8080 MAGIC_API_KEY=your-key python -m ecomops.seed_knowledge
# hoặc nạp chính sách thật của shop:
python -m ecomops.seed_knowledge /duong/dan/chinh-sach-shop.yaml
```

```python
from ecomops import MagicKnowledgeLookup
wf = EcomOpsWorkflow(..., knowledge_lookup=MagicKnowledgeLookup(MAGIC_URL, MAGIC_API_KEY))
```

**b) File YAML tại chỗ** (shop không chạy Knowledge Hub / core in-memory):

```python
from ecomops import StaticKnowledgeLookup
wf = EcomOpsWorkflow(..., knowledge_lookup=StaticKnowledgeLookup.from_yaml("chinh-sach-shop.yaml"))
```

Mỗi entry được gắn 2 tag: `ecomops` + topic (`shipping`, `return`, `order`,
`product`, `complaint`). Lookup lọc theo cả hai, nên câu hỏi về phí ship không bị
trả nhầm chính sách đổi trả, và cũng không kéo về kiến thức khác của tổ chức chỉ
vì trùng từ khoá.

> ⚠️ `sample_knowledge.yaml` là **dữ liệu mẫu để chạy thử**. Thay bằng chính sách
> thật của shop trước khi dùng với khách — nội dung sai ở đây sẽ thành câu trả lời sai.

## 5. Demo end-to-end (Zalo + Google Sheet)

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

## 6. Giới hạn hiện tại

- Từ khoá phân loại là **danh sách cố định trong code**, chưa cấu hình được qua file.
- Chưa có Case Management (tạo case khi khiếu nại/đổi trả) — thuộc Tuần 3–4 của roadmap.
- `example_worker.py` quét toàn bộ dải `Orders!A1:H500` mỗi lần tra đơn; với sheet
  lớn nên thay bằng Nhanh.vn connector hoặc thêm cache.
- Guardrail dựa trên cụm từ, không phải mô hình — bắt được các mẫu hứa hẹn phổ biến,
  nhưng không thay thế được người duyệt với các đơn giá trị cao.

## 7. Chạy test

```bash
pytest packs/ecomops/tests
```
