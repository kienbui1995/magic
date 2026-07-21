# Zalo OA Connector

Kết nối MagiC với **Zalo Official Account** — nhận tin nhắn người dùng gửi tới OA
(qua webhook) và gửi tin nhắn trả lời (text, template).

> Phiên bản cơ bản — đủ dùng để nhận/gửi tin nhắn text. Chưa xử lý toàn bộ loại
> event của Zalo (sticker, location, follow/unfollow...) hay rate limit chi tiết
> theo từng tier OA. Luôn kiểm tra lại với [tài liệu chính thức](https://developers.zalo.me)
> trước khi lên production — API của Zalo có thể thay đổi.

## 1. Chuẩn bị

1. Tạo app tại [developers.zalo.me](https://developers.zalo.me) → lấy `app_id`, `app_secret`.
2. Liên kết Official Account với app, lấy `access_token` + `refresh_token` ban đầu
   (qua OAuth flow — xem mục "Official Account Access Token" trong docs Zalo).
3. Vào trang cấu hình Webhook của OA (Official Account Manager) → lấy `oa_secret_key`,
   dùng để xác thực chữ ký webhook.
4. Copy `config.example.yaml` → `config.yaml`, điền các giá trị trên.

## 2. Cài đặt

```bash
pip install -e ../../sdk/python[connectors]
pip install -r requirements.txt
```

## 3. Gửi tin nhắn

```python
import asyncio
import yaml
from zalo.connector import ZaloConnector

async def main():
    config = yaml.safe_load(open("config.yaml"))
    async with ZaloConnector(config) as conn:
        await conn.execute("send_text_message", {"user_id": "ZALO_USER_ID", "text": "Xin chào!"})

asyncio.run(main())
```

## 4. Nhận webhook (tin nhắn người dùng gửi tới OA)

1. Đăng ký webhook URL của bạn (vd. `https://your-domain.com/webhook/zalo`) trong
   Official Account Manager.
2. Chạy webhook server local để test (dùng ngrok/cloudflared để expose ra internet):

```python
import yaml
from zalo.connector import ZaloConnector
from zalo.webhook import serve

config = yaml.safe_load(open("config.yaml"))
connector = ZaloConnector(config)

def on_event(records: list[dict]) -> None:
    for msg in records:
        print(f"Tin nhắn mới từ {msg['sender_id']}: {msg['text']}")
        # TODO: submit task tới MagiC (vd. capability "classify_intent"),
        # rồi gọi lại connector.execute("send_text_message", ...) để trả lời.

serve(connector, on_event, port=9100)
```

## 5. Chạy test

```bash
pytest tests/
```

## Giới hạn hiện tại

- Chỉ hỗ trợ tin nhắn text và template attachment cơ bản.
- Chưa cache/limit theo tier OA (Zalo giới hạn số tin CS gửi/ngày tùy loại tài khoản) —
  connector sẽ raise `RateLimitError` khi Zalo trả lỗi rate-limit, workflow gọi connector
  cần tự quyết định chờ/bỏ qua.
- Access token refresh dùng `refresh_token` một lần — nếu hết hạn refresh token
  (~3 tháng), cần lấy lại token thủ công qua OAuth flow.
