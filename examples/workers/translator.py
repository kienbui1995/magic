"""
Translator Worker — Translates text between languages.
Usage: python examples/workers/translator.py

Note: This is a demo using simple word replacement.
Replace with a real translation API for production.
"""
from magic_claw import Worker

worker = Worker(name="TranslatorBot", endpoint="http://localhost:9002")

TRANSLATIONS = {
    "hello": {"vi": "xin chao", "fr": "bonjour", "es": "hola", "ja": "konnichiwa"},
    "world": {"vi": "the gioi", "fr": "monde", "es": "mundo", "ja": "sekai"},
    "thank you": {"vi": "cam on", "fr": "merci", "es": "gracias", "ja": "arigatou"},
    "goodbye": {"vi": "tam biet", "fr": "au revoir", "es": "adios", "ja": "sayounara"},
}

@worker.capability("translate", description="Translate text to target language")
def translate(text: str, target_lang: str = "vi") -> dict:
    result = text.lower()
    for eng, translations in TRANSLATIONS.items():
        if target_lang in translations:
            result = result.replace(eng, translations[target_lang])
    return {
        "original": text,
        "translated": result,
        "target_lang": target_lang,
    }

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve(port=9002)
