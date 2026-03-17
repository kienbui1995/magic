"""
Generator Worker — Generates content from templates.
Usage: python examples/workers/generator.py

Note: This is a demo using string templates.
Replace with an LLM for production.
"""
from magic_ai_sdk import Worker

worker = Worker(name="GeneratorBot", endpoint="http://localhost:9005")

TEMPLATES = {
    "blog_intro": "In today's rapidly evolving landscape of {topic}, professionals are seeking new ways to {goal}. This article explores the key trends and actionable strategies that can help you stay ahead.",
    "email_subject": "[Action Required] {topic} — Important Update for Your Team",
    "tweet": "{topic} is changing everything. Here's what you need to know: {key_point} #AI #Tech",
    "product_description": "Introducing our latest solution for {topic}. Designed for teams who need {goal}, it delivers results from day one.",
}

@worker.capability("generate", description="Generate content from templates")
def generate(template: str = "blog_intro", topic: str = "AI", goal: str = "improve efficiency", key_point: str = "") -> dict:
    tmpl = TEMPLATES.get(template, TEMPLATES["blog_intro"])
    content = tmpl.format(topic=topic, goal=goal, key_point=key_point)
    return {
        "content": content,
        "template_used": template,
        "word_count": len(content.split()),
    }

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve(port=9005)
