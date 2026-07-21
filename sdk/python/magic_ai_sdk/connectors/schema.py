"""Common Data Schema — the shared shape every connector maps to/from.

Connectors never expose platform-specific data to workflows directly;
they translate to and from these models so a workflow written against
"Conversation" or "Order" works the same whether the data came from
Zalo, Google Sheet, Nhanh.vn, or Base.vn.
"""

from datetime import datetime
from enum import Enum
from typing import Any, Optional

from pydantic import BaseModel, Field


class ChannelType(str, Enum):
    ZALO = "zalo"
    FACEBOOK = "facebook"
    GOOGLE_SHEET = "google_sheet"
    NHANH = "nhanh"
    BASE = "base"
    MISA = "misa"


class Customer(BaseModel):
    id: str
    source_connector: str
    external_id: str
    name: Optional[str] = None
    phone: Optional[str] = None
    email: Optional[str] = None
    address: Optional[str] = None
    metadata: dict[str, Any] = Field(default_factory=dict)


class OrderStatus(str, Enum):
    PENDING = "pending"
    CONFIRMED = "confirmed"
    SHIPPING = "shipping"
    DELIVERED = "delivered"
    CANCELLED = "cancelled"
    RETURNED = "returned"


class OrderItem(BaseModel):
    sku: Optional[str] = None
    name: str
    quantity: int = Field(default=1, ge=1)
    unit_price: float = 0.0


class Order(BaseModel):
    id: str
    source_connector: str
    external_id: str
    customer_id: Optional[str] = None
    status: OrderStatus = OrderStatus.PENDING
    items: list[OrderItem] = Field(default_factory=list)
    total_amount: float = 0.0
    shipping_fee: float = 0.0
    created_at: Optional[datetime] = None
    metadata: dict[str, Any] = Field(default_factory=dict)


class MessageDirection(str, Enum):
    INBOUND = "inbound"
    OUTBOUND = "outbound"


class Message(BaseModel):
    id: str
    conversation_id: str
    direction: MessageDirection
    sender_id: str
    text: Optional[str] = None
    attachments: list[dict[str, Any]] = Field(default_factory=list)
    sent_at: Optional[datetime] = None
    metadata: dict[str, Any] = Field(default_factory=dict)


class Conversation(BaseModel):
    id: str
    source_connector: str
    channel: ChannelType
    external_id: str
    customer_id: Optional[str] = None
    messages: list[Message] = Field(default_factory=list)
    metadata: dict[str, Any] = Field(default_factory=dict)


class CaseStatus(str, Enum):
    OPEN = "open"
    IN_PROGRESS = "in_progress"
    RESOLVED = "resolved"
    ESCALATED = "escalated"


class Case(BaseModel):
    id: str
    source_connector: str
    conversation_id: Optional[str] = None
    customer_id: Optional[str] = None
    title: str
    status: CaseStatus = CaseStatus.OPEN
    reason: Optional[str] = None
    metadata: dict[str, Any] = Field(default_factory=dict)


class KnowledgeDocument(BaseModel):
    id: str
    title: str
    content: str
    tags: list[str] = Field(default_factory=list)
    metadata: dict[str, Any] = Field(default_factory=dict)
