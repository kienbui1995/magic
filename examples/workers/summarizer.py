"""
Summarizer Worker — Summarizes text content.
Usage: python examples/workers/summarizer.py
"""
from magic_claw import Worker

worker = Worker(name="SummarizerBot", endpoint="http://localhost:9001")

@worker.capability("summarize", description="Summarize text content into key points")
def summarize(text: str, max_points: int = 3) -> dict:
    sentences = [s.strip() for s in text.split('.') if s.strip()]
    points = sentences[:max_points]
    return {
        "summary": ". ".join(points) + ".",
        "point_count": len(points),
        "original_length": len(text),
    }

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve(port=9001)
