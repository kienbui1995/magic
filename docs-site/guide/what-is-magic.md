# What is MagiC?

MagiC is an open-source framework for **managing fleets of AI workers**. Think Kubernetes for AI agents — it doesn't build agents, it manages any agents built with any tool through an open protocol.

## The Problem

Most AI frameworks force you to build agents in their way:
- **CrewAI** — Python only, tightly coupled roles
- **AutoGen** — Python only, GroupChat model
- **LangGraph** — Python only, graph model

None of them answer: *what happens when you need 10 workers in production, each built differently, and you want to monitor costs, route tasks intelligently, and handle failures?*

## The Solution

MagiC is the **orchestration layer above your agents**:

```
         You (CEO)
              │
        MagiC Server           ← routing, monitoring, cost control
       /    │    │    \
  CrewAI  LangChain  Custom   ← any framework, any language
  Agent    Chain     Bot
```

Your CrewAI agent becomes a MagiC worker. Your LangChain chain becomes a MagiC worker. They join the same organization and work together.

## Key Concepts

- **Workers** — any HTTP server that registers capabilities and accepts tasks
- **Tasks** — units of work routed to the best available worker
- **Workflows** — multi-step DAGs with parallel execution and failure handling
- **Organizations** — groups of workers with shared budgets and policies
- **Protocol (MCP²)** — JSON over HTTP; workers respond with `task.complete` or `task.fail`
