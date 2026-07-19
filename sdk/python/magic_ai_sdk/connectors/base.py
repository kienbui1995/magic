"""BaseConnector — the interface every platform integration implements.

A connector's job is narrow on purpose: authenticate, call the platform's
API (`execute`), and translate data to/from the Common Data Schema
(`map_to_common_schema` / `map_from_common_schema`). Business logic
(intent classification, guardrails, case management...) belongs in the
workflow that calls the connector, not in the connector itself — that's
what keeps magic core platform-agnostic.
"""

from abc import ABC, abstractmethod
from typing import Any, Optional


class BaseConnector(ABC):
    """Abstract base class for every connector (Zalo, Nhanh, Base, ...)."""

    name: str
    version: str = "0.1.0"

    def __init__(self, config: dict[str, Any]):
        self.config = config

    @abstractmethod
    async def connect(self) -> bool:
        """Authenticate / establish connection with the external platform."""

    @abstractmethod
    async def disconnect(self) -> None:
        """Release any held resources (sessions, connections)."""

    @abstractmethod
    async def execute(self, operation: str, params: dict[str, Any]) -> Any:
        """Perform a platform operation, e.g. `send_text_message`, `get_order`."""

    @abstractmethod
    def map_to_common_schema(self, raw_data: Any, entity_type: str) -> dict[str, Any]:
        """Translate a platform payload into the Common Data Schema."""

    @abstractmethod
    def map_from_common_schema(self, common_data: dict[str, Any], entity_type: str) -> Any:
        """Translate a Common Data Schema record into the platform's format."""

    async def health_check(self) -> bool:
        """Cheap liveness check. Override for a real ping if the platform supports one."""
        return True

    def verify_webhook_signature(self, headers: dict[str, str], raw_body: bytes) -> bool:
        """Validate an inbound webhook's signature. Override for webhook-based connectors."""
        raise NotImplementedError(f"{self.name} does not support webhooks")

    def parse_webhook_event(self, payload: dict[str, Any]) -> list[dict[str, Any]]:
        """Parse a verified webhook payload into a list of Common Schema records."""
        raise NotImplementedError(f"{self.name} does not support webhooks")

    async def __aenter__(self) -> "BaseConnector":
        await self.connect()
        return self

    async def __aexit__(self, exc_type, exc, tb) -> None:
        await self.disconnect()
