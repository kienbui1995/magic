from magic_ai_sdk import Worker

worker = Worker(name="HelloBot", endpoint="http://localhost:9000")

@worker.capability("greeting", description="Says hello to anyone")
def greet(name: str) -> str:
    return f"Hello, {name}! I'm managed by MagiC."

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve()
