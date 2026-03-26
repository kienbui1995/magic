package main

import (
	"fmt"

	magic "github.com/kienbui1995/magic/sdk/go"
)

func main() {
	w := magic.NewWorker("HelloBot-Go", "http://localhost:9001", 5)

	w.Capability("greeting", "Says hello to anyone", 0.0, func(input map[string]any) (map[string]any, error) {
		name, _ := input["name"].(string)
		return map[string]any{"result": fmt.Sprintf("Hello, %s! I'm a Go worker managed by MagiC.", name)}, nil
	})

	if err := w.Register("http://localhost:8080", ""); err != nil {
		panic(err)
	}
	if err := w.Serve(":9001"); err != nil {
		panic(err)
	}
}
