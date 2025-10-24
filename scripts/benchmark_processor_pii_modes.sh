#!/usr/bin/env bash
# Processor PII Mode Comparison Benchmark Script
# Compares enabled vs disabled PII redaction in the processor

set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_DIR="$REPO_ROOT/benchmarks/pii_processor"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$RESULTS_DIR"

echo "========================================"
echo "Processor PII Mode Comparison Benchmarks"
echo "========================================"
echo "Comparing: Hybrid Tokenizer/Regex redaction enabled vs disabled"
echo ""

cd "$REPO_ROOT/pkg/logs/processor"

# Run processor mode comparison benchmarks
echo "Running processor mode benchmarks (10 iterations, 10s each)..."
echo ""

go test -tags=test \
    -bench='BenchmarkProcessorPIIModes' \
    -run='^$' \
    -benchmem \
    -benchtime=5s \
    -count=5 \
    | tee "$RESULTS_DIR/processor_modes_$TIMESTAMP.txt"

echo ""
echo "========================================"
echo "Benchmarks Complete!"
echo "========================================"
echo ""
echo "Results saved to:"
echo "  $RESULTS_DIR/processor_modes_$TIMESTAMP.txt"
echo ""
echo "========================================"