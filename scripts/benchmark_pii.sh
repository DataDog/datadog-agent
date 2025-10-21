#!/usr/bin/env bash
# Simple PII Redaction Benchmark Script

set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_DIR="$REPO_ROOT/benchmarks/pii"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$RESULTS_DIR"

echo "========================================"
echo "PII Redaction Benchmarks"
echo "========================================"
echo ""

cd "$REPO_ROOT/pkg/logs/processor"

# Run benchmarks
echo "Running benchmarks (10 iterations, 10s each)..."
echo ""

go test -tags=test \
    -bench=BenchmarkPIIRedaction \
    -benchmem \
    -benchtime=10s \
    -count=10 \
    | tee "$RESULTS_DIR/results_$TIMESTAMP.txt"

echo ""
echo "========================================"
echo "Results saved to:"
echo "  $RESULTS_DIR/results_$TIMESTAMP.txt"
echo ""
echo "To analyze with benchstat:"
echo "  benchstat $RESULTS_DIR/results_$TIMESTAMP.txt"
echo ""
echo "To generate CPU profile:"
echo "  go test -tags=test -bench=BenchmarkPIIRedaction -cpuprofile=cpu.prof"
echo "  go tool pprof -http=:8080 cpu.prof"
echo "========================================"
