#!/usr/bin/env bash
# Processor PII Mode Comparison Benchmark Script
# Compares regex vs hybrid PII redaction modes in the processor

set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_DIR="$REPO_ROOT/benchmarks/pii_processor"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$RESULTS_DIR"

echo "========================================"
echo "Processor PII Mode Comparison Benchmarks"
echo "========================================"
echo "Comparing: Regex vs Hybrid vs Disabled (baseline)"
echo ""

cd "$REPO_ROOT/pkg/logs/processor"

# Run processor mode comparison benchmarks
echo "Running processor mode benchmarks (10 iterations, 10s each)..."
echo "This includes both BenchmarkProcessorPIIModes and BenchmarkProcessorPIIModesFullMessage"
echo "This will take ~15-20 minutes..."
echo ""

go test -tags=test \
    -bench='BenchmarkProcessorPIIModes' \
    -run='^$' \
    -benchmem \
    -benchtime=10s \
    -count=10 \
    | tee "$RESULTS_DIR/processor_modes_$TIMESTAMP.txt"

echo ""
echo "========================================"
echo "Benchmarks Complete!"
echo "========================================"
echo ""
echo "Results saved to:"
echo "  $RESULTS_DIR/processor_modes_$TIMESTAMP.txt"
echo ""
echo "To analyze with benchstat:"
echo "  # Extract regex results"
echo "  grep '/regex-' $RESULTS_DIR/processor_modes_$TIMESTAMP.txt > $RESULTS_DIR/regex_$TIMESTAMP.txt"
echo ""
echo "  # Extract hybrid results"
echo "  grep '/hybrid-' $RESULTS_DIR/processor_modes_$TIMESTAMP.txt > $RESULTS_DIR/hybrid_$TIMESTAMP.txt"
echo ""
echo "  # Extract baseline (disabled) results"
echo "  grep '/disabled-' $RESULTS_DIR/processor_modes_$TIMESTAMP.txt > $RESULTS_DIR/disabled_$TIMESTAMP.txt"
echo ""
echo "  # Compare regex vs hybrid"
echo "  benchstat $RESULTS_DIR/regex_$TIMESTAMP.txt $RESULTS_DIR/hybrid_$TIMESTAMP.txt > $RESULTS_DIR/benchstat_$TIMESTAMP.txt"
echo ""
echo "  # View the comparison"
echo "  cat $RESULTS_DIR/benchstat_$TIMESTAMP.txt"
echo ""
echo "To analyze automatically:"
echo "  ./scripts/analyze_processor_pii_benchmarks.sh $RESULTS_DIR/processor_modes_$TIMESTAMP.txt"
echo "========================================"
