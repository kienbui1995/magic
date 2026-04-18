"""MagiC benchmark load generator.

Submits tasks to a running MagiC gateway at a configurable rate, polls each
task for completion, and emits a CSV plus a summary with p50/p95/p99 latency
and throughput.

Example:
    python3 load.py --rate 100 --duration 60 --out run.csv
    python3 load.py --rate 0 --total 10000 --concurrency 200 --out bulk.csv

Designed to be self-contained: only depends on httpx (for async HTTP) and the
Python stdlib.
"""

from __future__ import annotations

import argparse
import asyncio
import csv
import statistics
import sys
import time
from dataclasses import dataclass, field
from typing import Optional

try:
    import httpx
except ImportError:  # pragma: no cover - runtime error path
    print("ERROR: httpx not installed. Run: pip install httpx", file=sys.stderr)
    sys.exit(1)


@dataclass
class Sample:
    task_id: str
    submit_ms: float
    complete_ms: Optional[float]
    status: str
    scheduled_at: float = 0.0


@dataclass
class Config:
    base_url: str
    token: str
    task_type: str
    rate: float  # rps; 0 means unlimited
    duration: float  # seconds; 0 means until --total reached
    total: int  # 0 means until --duration elapses
    concurrency: int
    drain: float
    out: str


async def submit_one(client: httpx.AsyncClient, cfg: Config) -> tuple[str, float]:
    """Submit one task. Returns (task_id, submit_latency_ms)."""
    payload = {
        "type": cfg.task_type,
        "input": {"echo": "bench"},
        "routing": {"strategy": "best_match", "required_capabilities": [cfg.task_type]},
    }
    t0 = time.perf_counter()
    r = await client.post(
        f"{cfg.base_url}/api/v1/tasks",
        json=payload,
        headers={"Authorization": f"Bearer {cfg.token}"},
    )
    submit_ms = (time.perf_counter() - t0) * 1000.0
    r.raise_for_status()
    return r.json()["id"], submit_ms


async def poll_complete(
    client: httpx.AsyncClient, cfg: Config, task_id: str, deadline: float
) -> str:
    """Poll until terminal status or deadline. Returns final status string."""
    while time.monotonic() < deadline:
        r = await client.get(
            f"{cfg.base_url}/api/v1/tasks/{task_id}",
            headers={"Authorization": f"Bearer {cfg.token}"},
        )
        if r.status_code == 200:
            status = r.json().get("status", "")
            if status in ("completed", "failed", "dlq"):
                return "ok" if status == "completed" else status
        await asyncio.sleep(0.05)
    return "timeout"


async def run_one(
    client: httpx.AsyncClient,
    cfg: Config,
    sem: asyncio.Semaphore,
    scheduled_at: float,
    samples: list[Sample],
) -> None:
    async with sem:
        submit_start = time.perf_counter()
        try:
            task_id, submit_ms = await submit_one(client, cfg)
        except Exception as exc:  # pylint: disable=broad-except
            samples.append(
                Sample(
                    task_id="-",
                    submit_ms=(time.perf_counter() - submit_start) * 1000.0,
                    complete_ms=None,
                    status=f"submit_err:{type(exc).__name__}",
                    scheduled_at=scheduled_at,
                )
            )
            return
        deadline = time.monotonic() + 30.0
        status = await poll_complete(client, cfg, task_id, deadline)
        complete_ms = (time.perf_counter() - submit_start) * 1000.0
        samples.append(
            Sample(
                task_id=task_id,
                submit_ms=submit_ms,
                complete_ms=complete_ms,
                status=status,
                scheduled_at=scheduled_at,
            )
        )


async def run_load(cfg: Config) -> list[Sample]:
    samples: list[Sample] = []
    sem = asyncio.Semaphore(cfg.concurrency)
    async with httpx.AsyncClient(timeout=30.0) as client:
        tasks: list[asyncio.Task] = []
        start = time.monotonic()
        i = 0
        interval = 1.0 / cfg.rate if cfg.rate > 0 else 0.0
        while True:
            now = time.monotonic() - start
            if cfg.total and i >= cfg.total:
                break
            if cfg.duration and now >= cfg.duration:
                break
            scheduled = start + (i * interval if interval else now)
            if interval:
                wait = scheduled - time.monotonic()
                if wait > 0:
                    await asyncio.sleep(wait)
            tasks.append(
                asyncio.create_task(run_one(client, cfg, sem, scheduled, samples))
            )
            i += 1
        await asyncio.gather(*tasks, return_exceptions=True)
        if cfg.drain > 0:
            await asyncio.sleep(cfg.drain)
    return samples


def percentile(data: list[float], p: float) -> float:
    if not data:
        return 0.0
    s = sorted(data)
    k = (len(s) - 1) * p / 100.0
    f, c = int(k), min(int(k) + 1, len(s) - 1)
    return s[f] + (s[c] - s[f]) * (k - f)


def write_csv(path: str, samples: list[Sample]) -> None:
    with open(path, "w", newline="") as fh:
        w = csv.writer(fh)
        w.writerow(["scheduled_at", "task_id", "submit_ms", "complete_ms", "status"])
        for s in samples:
            w.writerow(
                [
                    f"{s.scheduled_at:.6f}",
                    s.task_id,
                    f"{s.submit_ms:.3f}",
                    f"{s.complete_ms:.3f}" if s.complete_ms is not None else "",
                    s.status,
                ]
            )


def summarise(samples: list[Sample], wall_seconds: float) -> None:
    ok = [s for s in samples if s.status == "ok" and s.complete_ms is not None]
    total = len(samples)
    if not samples:
        print("no samples", file=sys.stderr)
        return
    lat = [s.complete_ms for s in ok]  # type: ignore[misc]
    print()
    print(f"Total submitted   : {total}")
    print(f"Success           : {len(ok)} ({100.0 * len(ok) / total:.2f}%)")
    print(f"Wall time         : {wall_seconds:.2f}s")
    print(f"Throughput (ok/s) : {len(ok) / wall_seconds:.2f}")
    if lat:
        print(f"Latency p50  (ms) : {percentile(lat, 50):.2f}")
        print(f"Latency p95  (ms) : {percentile(lat, 95):.2f}")
        print(f"Latency p99  (ms) : {percentile(lat, 99):.2f}")
        print(f"Latency max  (ms) : {max(lat):.2f}")
        print(f"Latency mean (ms) : {statistics.fmean(lat):.2f}")


def parse_args() -> Config:
    p = argparse.ArgumentParser(description="MagiC load generator")
    p.add_argument("--base-url", default="http://localhost:8080")
    p.add_argument("--token", default="dev-token")
    p.add_argument("--task-type", default="echo")
    p.add_argument("--rate", type=float, default=100.0, help="rps (0 = unlimited)")
    p.add_argument("--duration", type=float, default=0.0, help="seconds (0 = until --total)")
    p.add_argument("--total", type=int, default=0, help="total tasks (0 = until --duration)")
    p.add_argument("--concurrency", type=int, default=50)
    p.add_argument("--drain", type=float, default=0.0, help="seconds to wait after last submit")
    p.add_argument("--out", default="load.csv")
    a = p.parse_args()
    if not a.duration and not a.total:
        a.duration = 30.0
    return Config(
        base_url=a.base_url.rstrip("/"),
        token=a.token,
        task_type=a.task_type,
        rate=a.rate,
        duration=a.duration,
        total=a.total,
        concurrency=a.concurrency,
        drain=a.drain,
        out=a.out,
    )


def main() -> None:
    cfg = parse_args()
    start = time.monotonic()
    samples = asyncio.run(run_load(cfg))
    wall = time.monotonic() - start
    write_csv(cfg.out, samples)
    print(f"Wrote {cfg.out} ({len(samples)} rows)")
    summarise(samples, wall)


if __name__ == "__main__":
    main()
