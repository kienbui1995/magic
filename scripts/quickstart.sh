#!/usr/bin/env bash
# 1-click local dev: start magic + postgres + redis, wait for health, print next steps.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> Starting magic + postgres + redis (docker compose)..."
docker compose up -d --build

echo "==> Waiting for magic to become healthy..."
for _ in $(seq 1 30); do
  if curl -sf http://localhost:8080/health > /dev/null 2>&1; then
    echo "==> magic is up: http://localhost:8080"
    echo ""
    echo "Next steps:"
    echo "  - Register a worker: see examples/hello-worker/main.py"
    echo "  - Connector Framework: see connectors/README.md"
    echo "  - Logs: docker compose logs -f magic"
    echo "  - Stop: docker compose down"
    exit 0
  fi
  sleep 2
done

echo "magic did not become healthy in time — check logs: docker compose logs magic" >&2
exit 1
