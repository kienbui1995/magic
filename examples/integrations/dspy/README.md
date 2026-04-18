# DSPy + MagiC

Expose a **DSPy** program — zero-shot or compiled — as a MagiC worker
capability. Versioned prompts, A/B testing, and cost tracking per variant,
without wiring any infra yourself.

## Why it matters

DSPy treats prompts as code you compile, not strings you hand-tune. That
model is powerful, but it raises a deployment question: once you have
three compiled variants of the same program, how do you route traffic
between them, measure cost per variant, and roll back if the new one
regresses?

MagiC answers that at the fleet layer. Register each DSPy program (or
each compiled variant) as a worker with capability `classify_intent`,
and the router picks one by `best_match` / `round_robin` / `cheapest`.
CostCtrl already tracks spend per worker — so you get per-variant
economics for free. Audit logs capture every prediction.

**DSPy compiles the program. MagiC ships it, versions it, and measures it.**

## Architecture

```
    client (curl / SDK)
            │
            │  POST /api/v1/tasks  type=classify_intent
            ▼
    ┌────────────────────┐
    │   MagiC Gateway    │  auth, cost cap, policy
    └─────────┬──────────┘
              │  router picks variant (best_match / round_robin / cheapest)
              ▼
    ┌────────────────────────────────┐
    │   DSPyWorker (this file)       │
    │                                │
    │   IntentClassifier(dspy.Module)│
    │   └─ ChainOfThought            │
    │       └─ ClassifyIntent sig    │
    └────────────────────────────────┘
```

Run a second instance of this worker with a compiled program (e.g.
`BootstrapFewShot`) and the same capability name — MagiC will route
between them. Compare cost and quality from the metrics endpoint.

## Run

### 1. Set up a virtualenv and install deps

```bash
cd examples/integrations/dspy
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
```

### 2. Configure environment

```bash
cp .env.example .env
# edit .env — set OPENAI_API_KEY, or switch to Ollama (see .env.example)
```

### 3. Start MagiC gateway (in another terminal)

```bash
cd core
go build ./cmd/magic
./magic serve         # listens on :8080
```

### 4. Run the worker

```bash
python main.py
# → registers with MagiC and serves on :9106
```

### 5. Submit a task

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "classify_intent",
    "input": {"text": "I love how fast this framework is — shipped on Friday."},
    "routing": {"required_capabilities": ["classify_intent"]},
    "contract": {"timeout_ms": 30000, "max_cost": 0.05}
  }'
```

Response shape:

```json
{
  "status": "done",
  "output": {
    "label": "positive",
    "reasoning": "The text expresses enthusiasm about speed and shipping.",
    "model": "openai/gpt-4o-mini"
  }
}
```

## What you get (for free)

| Feature | Provided by MagiC |
|---|---|
| Per-variant cost tracking (A vs B compiled program) | CostCtrl |
| Router strategies for A/B (round_robin, cheapest, best_match) | Router |
| Budget caps per team per day | CostCtrl |
| RBAC on who can hit the classifier | OrgMgr |
| Audit log of every classification | Audit |
| Retry on transient LLM errors | Router |
| Prometheus metrics per DSPy program variant | Monitor |

## Local-only mode (no OpenAI bill)

DSPy routes through LiteLLM under the hood, so Ollama works with one
env tweak:

```env
OPENAI_API_KEY=ollama
OPENAI_API_BASE=http://localhost:11434/v1
DSPY_LLM_MODEL=ollama_chat/llama3.2
```

Smaller local models may ignore the label constraint; the handler
normalises any out-of-set output to `neutral` defensively.

## Upgrading to a compiled variant

Inside `_program_singleton()`:

```python
from dspy.teleprompt import BootstrapFewShot
trainset = [dspy.Example(text="...", label="positive", reasoning="...").with_inputs("text"), ...]
_program = BootstrapFewShot(metric=my_metric).compile(IntentClassifier(), trainset=trainset)
```

Ship that worker side-by-side with the zero-shot one on the same
capability name and let MagiC route between them.

## Pattern

```python
from magic_ai_sdk import Worker
import dspy

dspy.configure(lm=dspy.LM(model="openai/gpt-4o-mini"))
program = MyDspyProgram()                     # <-- your existing program

worker = Worker(name="MyDSPy", endpoint="http://localhost:9106")

@worker.capability("my_dspy_task")
def run(**inputs) -> dict:
    pred = program(**inputs)
    return {"label": pred.label, "reasoning": pred.reasoning}

worker.run("http://localhost:8080", port=9106)
```

That is the whole integration.

## Disclaimer

DSPy is still iterating quickly; `dspy.configure`, `dspy.LM`, and
`dspy.Signature` all landed in 2.5 and minor releases can rename
fields. If the example fails to import, pin `dspy-ai>=2.5,<2.7`.
