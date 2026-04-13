#!/usr/bin/env python3
"""
Post MagiC to Reddit via OAuth (no password needed).
Works on remote servers — paste redirect URL manually.
"""
import os
import urllib.request
import urllib.parse
import json
import base64
import time

CLIENT_ID = os.environ.get("REDDIT_CLIENT_ID", "")
CLIENT_SECRET = os.environ.get("REDDIT_CLIENT_SECRET", "")
REDIRECT_URI = "http://localhost:8080"

if not CLIENT_ID or not CLIENT_SECRET:
    print("Set env vars:")
    print("  export REDDIT_CLIENT_ID='your_client_id'")
    print("  export REDDIT_CLIENT_SECRET='your_secret'")
    exit(1)

# --- Step 1: User authorizes in browser ---
state = "magic_post"
auth_url = (
    f"https://www.reddit.com/api/v1/authorize?"
    f"client_id={CLIENT_ID}&response_type=code&state={state}"
    f"&redirect_uri={REDIRECT_URI}&duration=temporary&scope=submit"
)

print("1. Open this URL in your browser:\n")
print(f"   {auth_url}\n")
print("2. Click 'Allow' on Reddit")
print("3. Browser will show 'can't connect' — that's OK!")
print("4. Copy the FULL URL from browser address bar and paste here:\n")

redirect_url = input("Paste URL: ").strip()

# Extract code from URL
params = urllib.parse.parse_qs(urllib.parse.urlparse(redirect_url).query)
auth_code = params.get("code", [None])[0]

if not auth_code:
    print("No authorization code found in URL.")
    exit(1)
print(f"\nCode received!\n")

# --- Step 2: Exchange code for token ---
print("Getting access token...")
auth = base64.b64encode(f"{CLIENT_ID}:{CLIENT_SECRET}".encode()).decode()
data = urllib.parse.urlencode({
    "grant_type": "authorization_code",
    "code": auth_code,
    "redirect_uri": REDIRECT_URI,
}).encode()

req = urllib.request.Request("https://www.reddit.com/api/v1/access_token", data=data, method="POST")
req.add_header("Authorization", f"Basic {auth}")
req.add_header("User-Agent", f"python:magic-poster:v1.0 (by /u/Individual_Muscle424)")

resp = urllib.request.urlopen(req)
token_data = json.loads(resp.read())
if "access_token" not in token_data:
    print(f"Token exchange failed: {token_data}")
    exit(1)

token = token_data["access_token"]

# Get username
req = urllib.request.Request("https://oauth.reddit.com/api/v1/me")
req.add_header("Authorization", f"Bearer {token}")
req.add_header("User-Agent", "python:magic-poster:v1.0 (by /u/Individual_Muscle424)")
resp = urllib.request.urlopen(req)
username = json.loads(resp.read())["name"]
print(f"Authenticated as u/{username}\n")

# --- Step 3: Post ---
POSTS = [
    {
        "subreddit": "golang",
        "title": 'MagiC \u2014 an open-source "Kubernetes for AI agents" written in Go',
        "text": (
            "I built MagiC, an open-source framework for managing fleets of AI agents. "
            "Go core, zero external dependencies, 90 tests with -race.\n\n"
            "The idea: everyone's building AI agents (CrewAI, LangChain, etc.) but nobody "
            "is managing them when you have 10+ running together. MagiC handles routing, "
            "cost control, DAG workflows, and circuit breakers.\n\n"
            "- 9 modules, single binary\n"
            "- Bounded event bus (worker pool pattern)\n"
            "- Circuit breaker for worker failures\n"
            "- SQLite persistent storage\n"
            "- Python SDK for building workers\n\n"
            "GitHub: https://github.com/kienbui1995/magic\n"
            "Blog: https://dev.to/kienbui1995/why-i-built-magic-and-why-managing-ai-agents-is-the-real-problem-37i9\n\n"
            "Would love feedback on the Go architecture. Apache 2.0."
        ),
    },
    {
        "subreddit": "AI_Agents",
        "title": "I built an open-source framework for managing AI agents \u2014 like Kubernetes but for agents",
        "text": (
            'After reading about "agent sprawl" (CIO Magazine) and Deloitte predicting '
            "$8.5B AI agent market in 2026, I built MagiC \u2014 a framework that manages "
            "any AI agent from any framework.\n\n"
            "The problem: CrewAI, AutoGen, LangGraph all help you BUILD agents. "
            "But when you have 10+ agents, who manages them?\n\n"
            "MagiC handles:\n"
            "- Capability-based routing (best match, cheapest)\n"
            "- Cost tracking with budget alerts + auto-pause\n"
            "- DAG workflows with parallel execution\n"
            "- Human-in-the-loop approval gates\n"
            "- Circuit breaker for failing workers\n\n"
            "A worker is 10 lines of Python. Works with CrewAI, LangChain, or plain Python.\n\n"
            "GitHub: https://github.com/kienbui1995/magic\n"
            "Blog: https://dev.to/kienbui1995/why-i-built-magic-and-why-managing-ai-agents-is-the-real-problem-37i9"
        ),
    },
    {
        "subreddit": "opensource",
        "title": "MagiC \u2014 open-source framework for managing fleets of AI agents (Go + Python, Apache 2.0)",
        "text": (
            "Just open-sourced MagiC \u2014 a framework for orchestrating AI agents. "
            "Think Kubernetes for AI workers.\n\n"
            "Go core (zero deps, 90 tests), Python SDK, SQLite storage. "
            "Any agent from any framework can register as a worker.\n\n"
            "GitHub: https://github.com/kienbui1995/magic\n"
            "Landing: https://kienbui1995.github.io/magic/\n\n"
            "Apache 2.0. Feedback welcome!"
        ),
    },
]

for i, post in enumerate(POSTS):
    print(f"Posting to r/{post['subreddit']}...")
    data = urllib.parse.urlencode({
        "sr": post["subreddit"],
        "kind": "self",
        "title": post["title"],
        "text": post["text"],
    }).encode()

    req = urllib.request.Request("https://oauth.reddit.com/api/submit", data=data)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("User-Agent", f"magic-poster/1.0 by /u/{username}")

    try:
        resp = urllib.request.urlopen(req)
        result = json.loads(resp.read())
        url = result.get("json", {}).get("data", {}).get("url", "")
        if url:
            print(f"  Done: {url}")
        else:
            errors = result.get("json", {}).get("errors", [])
            if errors:
                print(f"  Error: {errors}")
            else:
                print(f"  Response: {json.dumps(result)[:300]}")
    except urllib.error.HTTPError as e:
        print(f"  Error {e.code}: {e.read().decode()[:200]}")

    if i < len(POSTS) - 1:
        print("  Waiting 30s...")
        time.sleep(30)

print("\nAll done!")
