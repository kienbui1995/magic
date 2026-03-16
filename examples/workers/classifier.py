"""
Classifier Worker — Classifies text into categories.
Usage: python examples/workers/classifier.py

Note: This is a demo using keyword matching.
Replace with an LLM or ML model for production.
"""
from magic_claw import Worker

worker = Worker(name="ClassifierBot", endpoint="http://localhost:9003")

CATEGORIES = {
    "tech": ["ai", "software", "code", "api", "cloud", "data", "machine learning", "algorithm"],
    "business": ["revenue", "profit", "market", "customer", "sales", "growth", "strategy"],
    "health": ["medical", "health", "patient", "doctor", "treatment", "disease", "symptom"],
    "finance": ["stock", "investment", "bank", "loan", "credit", "budget", "tax"],
}

@worker.capability("classify", description="Classify text into categories")
def classify(text: str) -> dict:
    text_lower = text.lower()
    scores = {}
    for category, keywords in CATEGORIES.items():
        score = sum(1 for kw in keywords if kw in text_lower)
        if score > 0:
            scores[category] = score

    if not scores:
        return {"category": "general", "confidence": 0.0, "scores": {}}

    best = max(scores, key=scores.get)
    total = sum(scores.values())
    return {
        "category": best,
        "confidence": round(scores[best] / max(total, 1), 2),
        "scores": scores,
    }

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve(port=9003)
