// This module exists for reference. Benchmark source files are at ../core/benchmarks/
// because Go's internal package visibility requires them to be within the core module.
//
// To run benchmarks:
//   cd ../core && go test -bench=. -benchtime=5s -benchmem ./benchmarks/...

module github.com/kienbui1995/magic/benchmarks

go 1.24.0

require github.com/kienbui1995/magic/core v0.0.0

replace github.com/kienbui1995/magic/core => ../core
