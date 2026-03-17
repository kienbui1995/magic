from dataclasses import dataclass, field
from typing import Any

@dataclass
class Capability:
    name: str
    description: str = ""
    est_cost_per_call: float = 0.0

@dataclass
class RegisterPayload:
    name: str
    capabilities: list[dict]
    endpoint: dict
    limits: dict = field(default_factory=lambda: {"max_concurrent_tasks": 5})
    metadata: dict = field(default_factory=dict)
