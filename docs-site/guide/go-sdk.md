# Go SDK

```bash
go get github.com/kienbui1995/magic/sdk/go
```

## Worker

```go
package main

import (
    "fmt"
    magic "github.com/kienbui1995/magic/sdk/go"
)

func main() {
    w := magic.NewWorker("SummaryBot", "http://myhost:9001")

    w.AddCapability(magic.Capability{
        Name:        "summarize",
        Description: "Summarizes long text",
    })

    w.HandleFunc("summarize", func(input map[string]any) (any, error) {
        text := input["text"].(string)
        // your AI logic
        return fmt.Sprintf("Summary: %s", text[:50]+"..."), nil
    })

    w.Run("http://localhost:8080", "0.0.0.0:9001")
}
```

## Auto-discover capabilities

```go
// Discovers capabilities from worker endpoint automatically
err := w.DiscoverCapabilities("http://localhost:8080")
```
