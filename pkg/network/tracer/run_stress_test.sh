#!/bin/bash
# Stress test runner for TCPServer race condition
# Based on CI showing ~1.5-2% failure rate

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "========================================="
echo "TCPServer Stress Test Runner"
echo "========================================="
echo ""
echo "CI shows ~1.5-2% failure rate for TestTCPRTT"
echo "Running stress tests to reproduce..."
echo ""

# Check if fix is applied (only if we're in a git repo)
if git rev-parse --git-dir > /dev/null 2>&1; then
    if git diff --cached pkg/network/tracer/testutil/tcp.go 2>/dev/null | grep -q "wg.*WaitGroup"; then
        echo "⚠️  WARNING: Fix appears to be staged/applied"
        echo "   The test may not fail with the fix in place"
        echo ""
    fi
fi

echo "Available stress tests:"
echo "  0. TestTracerSuite/*/TestTCPRTTStressCI       - 2,000 iterations (CI-friendly, ~15 seconds)"
echo "  1. TestTracerSuite/*/TestTCPRTTStressFast     - 1,000 iterations (~7-8 seconds)"
echo "  2. TestTracerSuite/*/TestTCPRTTStressParallel - 250,000 iterations, 50 workers (~45 seconds)"
echo "  3. TestTracerSuite/*/TestTCPRTTStress         - 100,000 iterations, 20 workers (~40 seconds)"
echo ""

# Function to run a test
run_stress_test() {
    local test_name=$1
    local desc=$2
    
    echo "========================================="
    echo "Running: $test_name"
    echo "$desc"
    echo "========================================="
    echo ""
    
    start_time=$(date +%s)
    
    if go test -tags linux_bpf,test -v -run "$test_name" -timeout=30m 2>&1 | tee stress_test_output.log; then
        echo ""
        echo "✅ Test completed successfully (no failures detected)"
    else
        echo ""
        echo "❌ Test FAILED - race condition reproduced!"
        echo ""
        echo "Failure details:"
        grep -A 2 "FAILURE" stress_test_output.log || true
    fi
    
    end_time=$(date +%s)
    duration=$((end_time - start_time))
    
    echo ""
    echo "Duration: ${duration}s"
    echo ""
}

# Default: run CI test first
TEST_NAME="${1:-TestTracerSuite/eBPFless/TestTCPRTTStressCI}"

case "$TEST_NAME" in
    "ci"|"0")
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStressCI" "2,000 iterations (CI-friendly)"
        ;;
    "fast"|"1")
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStressFast" "1,000 iterations"
        ;;
    "parallel"|"2")
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStressParallel" "250,000 iterations with 50 workers"
        ;;
    "full"|"3")
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStress" "100,000 iterations with 20 workers"
        ;;
    "all")
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStressCI" "2,000 iterations (CI-friendly)"
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStressFast" "1,000 iterations"
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStressParallel" "250,000 iterations with 50 workers"
        run_stress_test "TestTracerSuite/eBPFless/TestTCPRTTStress" "100,000 iterations with 20 workers"
        ;;
    *)
        run_stress_test "$TEST_NAME" "Custom test"
        ;;
esac

echo "========================================="
echo "Stress test complete!"
echo "========================================="
echo ""
echo "To run different tests:"
echo "  $0 ci        # 2K iterations (CI-friendly, default, ~15s)"
echo "  $0 fast      # 1K iterations (~7-8s)"
echo "  $0 parallel  # 250K iterations with 50 workers (~45s)"
echo "  $0 full      # 100K iterations with 20 workers (~40s)"
echo "  $0 all       # Run all tests"
echo ""
echo "With 2% CI failure rate:"
echo "  - 2,000 iterations: >99.99% chance of catching failure"
echo "  - 1,000 iterations: >99.9999% chance"
echo "  - 250,000 iterations: ~100% chance"
echo "  - 100,000 iterations: ~100% chance"

