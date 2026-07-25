"""Vietnamese prompt library for EcomOps draft replies.

Kept as data (not f-strings scattered through the workflow) so a shop can tune
tone and policy wording without touching pipeline code — and so the guardrail
tests can assert against the exact instructions the model was given.
"""

from .intents import Intent

SYSTEM_PROMPT = """Bạn là nhân viên chăm sóc khách hàng của một shop bán hàng online tại Việt Nam.

Nguyên tắc bắt buộc:
- Trả lời ngắn gọn, lịch sự, xưng "shop" và gọi khách là "anh/chị".
- CHỈ dùng thông tin được cung cấp trong phần "Dữ liệu". Không được bịa thông tin đơn hàng,
  ngày giao, giá, hay chính sách.
- KHÔNG hứa chắc chắn về ngày giao hàng. Chỉ nói theo trạng thái thực tế của đơn.
- KHÔNG tự ý hứa hoàn tiền, giảm giá, tặng voucher hay miễn phí vận chuyển.
- KHÔNG nhắc tới số điện thoại, email hay thông tin của khách hàng khác.
- Nếu không đủ thông tin để trả lời, hãy nói sẽ chuyển cho nhân viên hỗ trợ kiểm tra giúp.

Trả lời bằng tiếng Việt, tối đa 4 câu."""


INTENT_INSTRUCTIONS: dict[Intent, str] = {
    Intent.ORDER_STATUS: (
        "Khách đang hỏi tình trạng đơn hàng. Dựa vào dữ liệu đơn hàng bên dưới, "
        "cho khách biết trạng thái hiện tại. Nếu không có dữ liệu đơn, hãy hỏi khách "
        "mã đơn hàng hoặc số điện thoại đặt hàng."
    ),
    Intent.SHIPPING_FEE: (
        "Khách đang hỏi phí vận chuyển. Trả lời theo đúng biểu phí trong dữ liệu. "
        "Nếu không có dữ liệu về phí ship, hãy nói sẽ nhờ nhân viên báo giá chính xác."
    ),
    Intent.RETURN_EXCHANGE: (
        "Khách muốn đổi/trả hàng. Nêu đúng điều kiện đổi trả có trong dữ liệu chính sách. "
        "Không tự cam kết chấp nhận đổi trả khi chưa kiểm tra đơn."
    ),
    Intent.PRODUCT_INFO: (
        "Khách hỏi thông tin sản phẩm. Chỉ trả lời theo dữ liệu sản phẩm được cung cấp. "
        "Nếu thiếu thông tin, hãy nói sẽ kiểm tra và phản hồi lại."
    ),
    Intent.GREETING: (
        "Khách mới chào hỏi. Chào lại ngắn gọn và hỏi shop có thể hỗ trợ gì cho khách."
    ),
    Intent.COMPLAINT: (
        "Khách đang khiếu nại. Xin lỗi chân thành, ghi nhận vấn đề, và cho biết sẽ chuyển "
        "nhân viên phụ trách xử lý. Không tự đưa ra phương án đền bù."
    ),
    Intent.OTHER: (
        "Chưa xác định được yêu cầu của khách. Hỏi lại khách một cách lịch sự để làm rõ."
    ),
}


def build_messages(
    intent: Intent,
    customer_message: str,
    order: dict | None = None,
    knowledge: list[str] | None = None,
) -> list[dict[str, str]]:
    """Assemble the chat messages for a draft reply.

    The order/knowledge blocks are rendered explicitly under a "Dữ liệu" heading
    that the system prompt refers to, so "only use the provided data" is an
    instruction the model can actually follow.
    """
    sections = [INTENT_INSTRUCTIONS.get(intent, INTENT_INSTRUCTIONS[Intent.OTHER])]

    data_parts = []
    if order:
        data_parts.append("Đơn hàng:\n" + _render_order(order))
    if knowledge:
        data_parts.append("Chính sách / thông tin shop:\n" + "\n".join(f"- {k}" for k in knowledge))
    sections.append("Dữ liệu:\n" + ("\n\n".join(data_parts) if data_parts else "(không có dữ liệu)"))

    sections.append(f"Tin nhắn của khách:\n{customer_message}")

    return [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": "\n\n".join(sections)},
    ]


_ORDER_LABELS = {
    "id": "Mã đơn",
    "status": "Trạng thái",
    "total_amount": "Tổng tiền",
    "shipping_fee": "Phí ship",
    "created_at": "Ngày đặt",
}


def _render_order(order: dict) -> str:
    lines = [f"- {label}: {order[key]}" for key, label in _ORDER_LABELS.items() if order.get(key)]
    items = order.get("items") or []
    if items:
        rendered = ", ".join(
            f"{it.get('name', '?')} x{it.get('quantity', 1)}" for it in items if isinstance(it, dict)
        )
        if rendered:
            lines.append(f"- Sản phẩm: {rendered}")
    return "\n".join(lines) if lines else "(không có thông tin)"
