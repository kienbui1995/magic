"""
Extractor Worker — Extracts structured data from text.
Usage: python examples/workers/extractor.py

Note: This is a demo using regex patterns.
Replace with an LLM for production.
"""
import re
from magic_ai import Worker

worker = Worker(name="ExtractorBot", endpoint="http://localhost:9004")

@worker.capability("extract", description="Extract structured data (emails, URLs, numbers) from text")
def extract(text: str) -> dict:
    emails = re.findall(r'[\w.+-]+@[\w-]+\.[\w.-]+', text)
    urls = re.findall(r'https?://[^\s<>"{}|\\^`\[\]]+', text)
    phones = re.findall(r'\+?[\d\s\-()]{7,15}', text)
    numbers = re.findall(r'\$?[\d,]+\.?\d*%?', text)

    return {
        "emails": emails,
        "urls": urls,
        "phones": [p.strip() for p in phones],
        "numbers": numbers,
        "total_found": len(emails) + len(urls) + len(phones) + len(numbers),
    }

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve(port=9004)
