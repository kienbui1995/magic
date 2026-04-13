# OpenClaw Worker

Connects an [OpenClaw](https://openclaw.ai) agent into MagiC so you can route tasks to it, track costs, and coordinate it alongside other agents (CrewAI, LangGraph, custom bots).

## Prerequisites

- MagiC server running (`./bin/magic serve`)
- OpenClaw installed and running (`npm install -g openclaw@latest && openclaw onboard --install-daemon`)
- Python SDK installed (`pip install magic-ai-sdk`)

## Setup

### 1. Enable OpenClaw HTTP API

In `~/.openclaw/openclaw.json`:
```json
{
  "gateway": {
    "http": {
      "endpoints": {
        "chatCompletions": {
          "enabled": true
        }
      }
    }
  }
}
```

Then restart: `openclaw gateway restart`

### 2. Get your OpenClaw token

Open `http://127.0.0.1:18789` → copy the access token from the Control UI.

### 3. Run the adapter

```bash
OPENCLAW_URL=http://127.0.0.1:18789 \
OPENCLAW_TOKEN=your-token-here \
MAGIC_URL=http://localhost:8080 \
python openclaw_worker.py
```

## Usage

Once running, submit tasks to MagiC and it routes to OpenClaw automatically:

```bash
# Ask OpenClaw anything via natural language
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "ask",
    "input": {"prompt": "What files are in my Downloads folder?"},
    "routing": {"strategy": "best_match", "required_capabilities": ["ask"]}
  }'

# Summarize text
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "summarize",
    "input": {"text": "Your long text here..."},
    "routing": {"strategy": "best_match", "required_capabilities": ["summarize"]}
  }'
```

## Capabilities

| Capability | Description | Input |
|------------|-------------|-------|
| `ask` | Natural language task (anything OpenClaw can do) | `prompt: str` |
| `summarize` | Summarize text | `text: str` |
| `analyze` | Extract insights from content | `content: str`, `question: str` |

## Mix with other workers

The real value: run OpenClaw alongside other agents in the same MagiC org.

```bash
# Terminal 1: MagiC server
./bin/magic serve

# Terminal 2: OpenClaw adapter (this file)
OPENCLAW_TOKEN=xxx python openclaw_worker.py

# Terminal 3: Your CrewAI agent or any other worker
python my_crewai_worker.py

# Now MagiC routes tasks across ALL of them
```
