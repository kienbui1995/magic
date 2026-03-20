"""MagiC Python SDK — Build AI workers for the MagiC framework."""

from magic_ai_sdk.worker import Worker
from magic_ai_sdk.client import MagiCClient, AsyncMagiCClient
from magic_ai_sdk.decorators import capability

__version__ = "0.2.0"
__all__ = ["Worker", "MagiCClient", "AsyncMagiCClient", "capability", "__version__"]
