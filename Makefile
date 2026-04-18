.PHONY: build test run dev clean bench bench-go bench-load

build:
	cd core && go build -o ../bin/magic ./cmd/magic

test:
	cd core && go test ./... -v

run: build
	./bin/magic serve

dev:
	cd core && go run ./cmd/magic serve

clean:
	rm -rf bin/

# ---- Benchmarks ----

# Run Go micro-benchmarks (dispatcher, router, store, events).
bench-go:
	cd core && go test -bench=. -benchmem ./benchmarks/...

# Run the Python end-to-end load generator. Requires a running gateway
# + registered workers (see benchmarks/scripts/docker-compose.bench.yml).
bench-load:
	python3 benchmarks/scripts/load.py --rate 100 --duration 60 --out benchmarks/results/load.csv

# Default bench target = Go micro-benchmarks only; the load test is opt-in
# because it needs a live stack and takes minutes to stabilise.
bench: bench-go
	@echo ""
	@echo "Run 'make bench-load' separately — it requires a running magic server."
