# MagiC Template Workers

Ready-to-use workers for common tasks. Each worker runs standalone.

## Quick Start

```bash
# Terminal 1: Start MagiC server
./bin/magic serve

# Terminal 2: Start any worker
pip install -e sdk/python
python examples/workers/summarizer.py
```

## Available Workers

| Worker | Port | Capability | Description |
|--------|------|-----------|-------------|
| SummarizerBot | 9001 | `summarize` | Summarize text into key points |
| TranslatorBot | 9002 | `translate` | Translate text between languages |
| ClassifierBot | 9003 | `classify` | Classify text into categories |
| ExtractorBot | 9004 | `extract` | Extract emails, URLs, numbers from text |
| GeneratorBot | 9005 | `generate` | Generate content from templates |

## Example: Submit a task

```bash
# Summarize
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"type":"summarize","input":{"text":"AI is transforming industries. Companies are adopting machine learning. Automation is increasing productivity. New jobs are being created.","max_points":2},"routing":{"strategy":"best_match","required_capabilities":["summarize"]},"contract":{"timeout_ms":30000}}'

# Classify
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"type":"classify","input":{"text":"The new AI algorithm improved our revenue by 30%"},"routing":{"strategy":"best_match","required_capabilities":["classify"]},"contract":{"timeout_ms":30000}}'
```

## Customization

These workers use simple Python logic for demo purposes. To use real AI:

```python
# Replace the handler with an LLM call
from openai import OpenAI

client = OpenAI()

@worker.capability("summarize")
def summarize(text: str) -> dict:
    response = client.chat.completions.create(
        model="gpt-4",
        messages=[{"role": "user", "content": f"Summarize: {text}"}]
    )
    return {"summary": response.choices[0].message.content}
```
