#!/usr/bin/env bash
# profile_pipeline.sh — Reproducible profiling baseline for the DogStatsD-to-Forwarder pipeline
#
# Usage:
#   ./scripts/profile_pipeline.sh [output_dir]
#
# Default output_dir: plans/
#
# Requirements:
#   - go tool (Go 1.21+)
#   - go install golang.org/x/perf/cmd/benchstat@latest  (for comparison)
#   - go tool pprof  (bundled with Go)

set -euo pipefail

OUTPUT_DIR="${1:-plans}"
mkdir -p "${OUTPUT_DIR}"

echo "==> Pipeline profiling baseline — $(date)"
echo "==> Output directory: ${OUTPUT_DIR}"

# --------------------------------------------------------------------------
# 1. Full pipeline benchmark suite — save to bench-baseline-forwarder.txt
#    Run with count=5 for statistical stability (benchstat needs ≥5 samples).
#    Uses DD_LOG_LEVEL=off to suppress runtime log noise from stdout.
# --------------------------------------------------------------------------
echo ""
echo "--- [1/4] Running full benchmark suite (count=5, all packages) ---"

DD_LOG_LEVEL=off go test \
  -tags test \
  -bench=. \
  -benchmem \
  -count=5 \
  ./comp/dogstatsd/server/... \
  ./pkg/aggregator/... \
  ./pkg/serializer/... \
  ./comp/forwarder/defaultforwarder/... \
  2>&1 | tee "${OUTPUT_DIR}/bench-baseline-forwarder.txt"

echo ""
echo "--- [2/4] Generating aggregator pprof profiles ---"

# Aggregator allocation profile (alloc_objects counts object count, not bytes)
DD_LOG_LEVEL=off go test \
  -tags test \
  -bench='BenchmarkTimeSamplerFlushCardinality/1000-contexts|BenchmarkTimeSamplerSampleTagCount|BenchmarkContextResolver1000' \
  -benchmem \
  -benchtime=2s \
  -count=1 \
  -memprofile="${OUTPUT_DIR}/mem.out" \
  -cpuprofile="${OUTPUT_DIR}/cpu.out" \
  ./pkg/aggregator \
  2>/dev/null

echo "  Memory profile: ${OUTPUT_DIR}/mem.out"
echo "  CPU profile:    ${OUTPUT_DIR}/cpu.out"

echo ""
echo "--- [3/4] Extracting top allocation sites ---"

echo "=== Top 20 allocation sites (alloc_objects) ===" > "${OUTPUT_DIR}/pprof-allocs.txt"
go tool pprof -alloc_objects -top -nodecount=20 "${OUTPUT_DIR}/mem.out" 2>/dev/null \
  >> "${OUTPUT_DIR}/pprof-allocs.txt" || echo "(pprof analysis failed)" >> "${OUTPUT_DIR}/pprof-allocs.txt"

echo ""
echo "--- [4/4] Extracting top CPU hotspots ---"

echo "=== Top 20 CPU hotspots ===" > "${OUTPUT_DIR}/pprof-cpu.txt"
go tool pprof -top -nodecount=20 "${OUTPUT_DIR}/cpu.out" 2>/dev/null \
  >> "${OUTPUT_DIR}/pprof-cpu.txt" || echo "(pprof analysis failed)" >> "${OUTPUT_DIR}/pprof-cpu.txt"

echo ""
echo "==> Done. Profiles saved to ${OUTPUT_DIR}/"
echo ""
echo "To compare baseline vs. optimised results:"
echo "  benchstat ${OUTPUT_DIR}/bench-baseline-forwarder.txt ${OUTPUT_DIR}/bench-final-forwarder.txt"
echo ""
echo "To interactively explore profiles:"
echo "  go tool pprof -alloc_objects ${OUTPUT_DIR}/mem.out"
echo "  go tool pprof ${OUTPUT_DIR}/cpu.out"
