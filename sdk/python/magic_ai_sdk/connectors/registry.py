"""ConnectorRegistry — register connector classes, get per-tenant instances.

One process may serve many tenants (shops); each tenant needs its own
connector instance (different Zalo OA access token, different Nhanh.vn
API key). The registry keys instances by `tenant_id:connector_name` so
tenants never share credentials or in-memory state.
"""

from typing import Any

from magic_ai_sdk.connectors.base import BaseConnector


class ConnectorRegistry:
    """Class-level registry: connector classes are process-global, instances are per-tenant."""

    _connectors: dict[str, type[BaseConnector]] = {}
    _instances: dict[str, BaseConnector] = {}

    @classmethod
    def register(cls, name: str, connector_class: type[BaseConnector]) -> None:
        cls._connectors[name] = connector_class

    @classmethod
    def unregister(cls, name: str) -> None:
        cls._connectors.pop(name, None)

    @classmethod
    def is_registered(cls, name: str) -> bool:
        return name in cls._connectors

    @classmethod
    def get(cls, name: str, tenant_id: str, config: dict[str, Any]) -> BaseConnector:
        if name not in cls._connectors:
            raise ValueError(f"Connector '{name}' is not registered")

        key = f"{tenant_id}:{name}"
        if key not in cls._instances:
            cls._instances[key] = cls._connectors[name](config)
        return cls._instances[key]

    @classmethod
    def evict(cls, name: str, tenant_id: str) -> None:
        """Drop a cached instance, e.g. after credentials rotate."""
        cls._instances.pop(f"{tenant_id}:{name}", None)

    @classmethod
    def reset(cls) -> None:
        """Clear all registrations and instances. Test-only."""
        cls._connectors.clear()
        cls._instances.clear()
