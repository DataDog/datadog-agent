#!/bin/bash

# Script to run JSON aggregator benchmarks multiple times for accurate results
# Usage: ./run_benchmarks.sh [number_of_runs] [output_file]

RUNS=${1:-10}
OUTPUT_FILE=${2:-"benchmark_results.txt"}
BENCHMARK_PATTERN="^(BenchmarkJSONAggregator_SingleLineJSON|BenchmarkJSONAggregator_MultilineJSON|BenchmarkJSONAggregator_ComplexNestedJSON|BenchmarkJSONAggregator_InvalidSingleLineJSON|BenchmarkJSONAggregator_UnbalancedBraces)$"

echo "Running benchmarks ${RUNS} times..."
echo "Output file: ${OUTPUT_FILE}"
echo ""

# Clear previous results
> "${OUTPUT_FILE}"

# Get current branch info
BRANCH=$(git rev-parse --abbrev-ref HEAD)
COMMIT=$(git rev-parse --short HEAD)

# Write header
echo "Branch: ${BRANCH}" | tee -a "${OUTPUT_FILE}"
echo "Commit: ${COMMIT}" | tee -a "${OUTPUT_FILE}"
echo "Date: $(date)" | tee -a "${OUTPUT_FILE}"
echo "Runs: ${RUNS}" | tee -a "${OUTPUT_FILE}"
echo "Platform: $(uname -s)/$(uname -m)" | tee -a "${OUTPUT_FILE}"
echo "" | tee -a "${OUTPUT_FILE}"

# Run benchmarks multiple times
for i in $(seq 1 ${RUNS}); do
    echo "Run ${i}/${RUNS}..."
    go test -benchmem -run='^$' \
        -bench "${BENCHMARK_PATTERN}" \
        -benchtime=3s \
        ./pkg/logs/internal/decoder/auto_multiline_detection >> "${OUTPUT_FILE}"

    if [ $? -ne 0 ]; then
        echo "Error running benchmark on iteration ${i}"
        exit 1
    fi
done

echo ""
echo "Done! Results saved to ${OUTPUT_FILE}"
echo ""
echo "To analyze results with benchstat:"
echo "  go install golang.org/x/perf/cmd/benchstat@latest"
echo "  benchstat ${OUTPUT_FILE}"
