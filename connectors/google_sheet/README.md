# Google Sheet Connector

Đọc/ghi dữ liệu đơn hàng (`Order`) và khách hàng (`Customer`) từ Google Sheet —
phù hợp cho shop nhỏ chưa dùng phần mềm quản lý bán hàng riêng.

## 1. Chuẩn bị

1. Tạo project trên [Google Cloud Console](https://console.cloud.google.com), bật **Google Sheets API**.
2. Tạo **Service Account** → tải file JSON key.
3. Mở Google Sheet cần dùng → **Share** với email của Service Account
   (dạng `xxx@project-id.iam.gserviceaccount.com`), quyền **Editor**.
4. Copy `config.example.yaml` → `config.yaml`, điền `spreadsheet_id` (lấy từ URL sheet)
   và nội dung JSON key.

## 2. Cài đặt

```bash
pip install -e ../../sdk/python[connectors]
pip install -r requirements.txt
```

## 3. Sử dụng

Sheet cần có dòng header ở hàng đầu tiên khớp với tên field, ví dụ tab `Customers`:

| id | name | phone | email | address |
|----|------|-------|-------|---------|
| C001 | Nguyễn Văn A | 0901234567 | a@example.com | 12 Lê Lợi, Q1 |

```python
import asyncio
import yaml
from google_sheet.connector import GoogleSheetConnector

async def main():
    config = yaml.safe_load(open("config.yaml"))
    async with GoogleSheetConnector(config) as conn:
        # Đọc toàn bộ khách hàng
        data = await conn.execute("read_range", {"range": "Customers!A1:E100"})
        header, *rows = data.get("values", [])
        customers = [conn.map_to_common_schema({"header": header, "row": r}, "customer") for r in rows]
        print(customers)

        # Thêm khách hàng mới
        row = conn.map_from_common_schema(
            {"id": "C002", "name": "Trần Thị B", "phone": "0909999999"}, "customer"
        )
        await conn.execute("append_row", {"range": "Customers!A1:E1", "values": [row]})

asyncio.run(main())
```

## 4. Các operation hỗ trợ (`execute(operation, params)`)

| operation | params | Mô tả |
|-----------|--------|-------|
| `read_range` | `range` (A1 notation, vd `"Orders!A1:H100"`) | Đọc dữ liệu thô |
| `update_range` | `range`, `values` (list of rows) | Ghi đè một vùng |
| `append_row` | `range`, `values` | Thêm dòng mới vào cuối bảng |
| `batch_update` | `updates: [{range, values}, ...]` | Ghi nhiều vùng trong 1 lần gọi API |

## 5. Giới hạn hiện tại

- Mapping Common Schema dựa vào tên cột trong header — đổi tên cột sẽ làm mapping sai,
  chưa hỗ trợ mapping tùy biến qua config.
- `Order.items` lưu dưới dạng JSON string trong cột `items_json` (Sheet không có kiểu
  dữ liệu lồng nhau).
- Google Sheets API có giới hạn quota (mặc định 60 request ghi/phút/user) —
  connector sẽ raise `RateLimitError` khi bị giới hạn, cần tự xử lý backoff ở tầng gọi.

## 6. Chạy test

```bash
pytest tests/
```
