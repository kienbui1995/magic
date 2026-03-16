# Contributing to MagiC

## Prerequisites

- Go 1.22+
- Python 3.11+ (for SDK)

## Getting Started

```bash
git clone https://github.com/kienbui1995/magic.git
cd magic

# Build and test Go
cd core && go build ./cmd/magic && go test ./... -v -race

# Python SDK
cd sdk/python && python -m venv .venv
.venv/bin/pip install -e ".[dev]" && .venv/bin/pytest tests/ -v
```

## Development Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/your-feature`
3. Make your changes with tests
4. Run: `cd core && go test ./... -race`
5. Commit with conventional commits: `feat(module): description`
6. Push and create a Pull Request

## Commit Convention

- `feat(scope): add new feature`
- `fix(scope): fix a bug`
- `docs: update documentation`
- `chore: maintenance tasks`

## Code Style

- Go: `gofmt`, godoc comments on exported types
- Python: PEP 8, type hints
- All tests must pass including `go test -race`

## Reporting Issues

Use GitHub Issues with: steps to reproduce, expected vs actual behavior, Go/Python version.
