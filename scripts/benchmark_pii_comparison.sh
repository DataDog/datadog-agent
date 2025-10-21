#!/usr/bin/env bash
# PII Detection Comparison Benchmark Script
# Compares regex, hybrid, and tokenization approaches

set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_DIR="$REPO_ROOT/benchmarks/pii"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$RESULTS_DIR"

echo "========================================"
echo "PII Detection Comparison Benchmarks"
echo "========================================"
echo "Comparing: Regex vs Hybrid vs Tokenization"
echo ""

cd "$REPO_ROOT/pkg/logs/processor"

# Run comparison benchmarks
echo "Running comparison benchmarks (10 iterations, 10s each)..."
echo "This will take ~5-10 minutes..."
echo ""

go test -tags=test \
    -bench=BenchmarkPIIDetectionComparison \
    -run='^$' \
    -benchmem \b
    -benchtime=10s \
    -count=10 \
    | tee "$RESULTS_DIR/comparison_$TIMESTAMP.txt"

echo ""
echo "========================================"
echo "Benchmarks Complete!"
echo "========================================"
echo ""
echo "Results saved to:"
echo "  $RESULTS_DIR/comparison_$TIMESTAMP.txt"
echo ""
echo "To analyze with benchstat:"
echo "  benchstat $RESULTS_DIR/comparison_$TIMESTAMP.txt"
echo ""
echo "To compare specific approaches:"
echo "  # Extract just regex results"
echo "  grep 'regex-' $RESULTS_DIR/comparison_$TIMESTAMP.txt > regex.txt"
echo "  # Extract just hybrid results"
echo "  grep 'hybrid-' $RESULTS_DIR/comparison_$TIMESTAMP.txt > hybrid.txt"
echo "  # Extract just tokenization results"
echo "  grep 'tokenization_only-' $RESULTS_DIR/comparison_$TIMESTAMP.txt > tokenization.txt"
echo "  # Compare regex vs hybrid"
echo "  benchstat regex.txt hybrid.txt"
echo ""
echo "To filter by message type:"
echo "  grep 'no_pii' $RESULTS_DIR/comparison_$TIMESTAMP.txt | benchstat"
echo "  grep 'single_email' $RESULTS_DIR/comparison_$TIMESTAMP.txt | benchstat"
echo "  grep 'multiple_pii_mixed' $RESULTS_DIR/comparison_$TIMESTAMP.txt | benchstat"
echo "========================================"

