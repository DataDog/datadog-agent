# TCPServer Flaky Test Stress Tests

This directory contains stress tests to reproduce the flaky `TestTCPRTT` failure observed in CI.

## Background

CI shows that `TestTracerSuite/eBPFless/TestTCPRTT` fails approximately **1.5-2%** of the time with the error:
```
write tcp 127.0.0.1:X->127.0.0.1:Y: write: bad file descriptor
```

This is caused by goroutine leaks in `TCPServer` leading to ephemeral port reuse conflicts between parallel tests.

## Quick Start

```bash
cd pkg/network/tracer

# CI-friendly test (200 iterations, ~30-60 seconds) - RUNS IN CI
./run_stress_test.sh ci

# Fast test (1,000 iterations, ~1 minute)
./run_stress_test.sh fast

# Full test (10,000 iterations, ~10 minutes)  
./run_stress_test.sh full

# Or run directly
go test -tags test -v -run TestTCPRTTStressCI -timeout=5m
```

## Test Variants

### 0. TestTCPRTTStressCI (✅ Runs in CI)
- **Iterations**: 2,000
- **Duration**: ~15 seconds
- **Success probability**: >99.99% chance of detecting a 2% failure rate
- **Use case**: CI validation - balances thoroughness with speed

```bash
sudo go test -tags linux_bpf,test -v -run TestTracerSuite/eBPFless/TestTCPRTTStressCI -timeout=5m
```

### 1. TestTCPRTTStressFast (Recommended for local quick validation)
- **Iterations**: 1,000
- **Duration**: ~7-8 seconds
- **Success probability**: >99.9999% chance of detecting a 2% failure rate
- **Use case**: Quick validation during development

```bash
sudo go test -tags linux_bpf,test -v -run TestTracerSuite/eBPFless/TestTCPRTTStressFast -timeout=5m
```

### 2. TestTCPRTTStressParallel
- **Iterations**: 250,000  
- **Parallelism**: 50 workers
- **Duration**: ~45 seconds
- **Success probability**: ~100% chance
- **Use case**: Medium confidence before committing

```bash
sudo go test -tags linux_bpf,test -v -run TestTracerSuite/eBPFless/TestTCPRTTStressParallel -timeout=5m
```

### 3. TestTCPRTTStress (Most thorough)
- **Iterations**: 100,000
- **Parallelism**: 20 workers  
- **Duration**: ~40 seconds
- **Success probability**: ~100% chance
- **Use case**: High confidence validation, pre-merge testing

```bash
sudo go test -tags linux_bpf,test -v -run TestTracerSuite/eBPFless/TestTCPRTTStress -timeout=5m
```

## Statistical Confidence

With a 2% CI failure rate:

| Iterations | Probability of Detecting Failure | Used For |
|-----------|----------------------------------|----------|
| 2,000     | >99.99%                          | CI (TestTCPRTTStressCI) |
| 1,000     | >99.9999%                        | Local fast |
| 250,000   | ~100%                            | Local parallel |
| 100,000   | ~100%                            | Local thorough |

Formula: `P(detect) = 1 - (1 - 0.02)^n`

## Testing Workflow

### Step 1: Reproduce the failure WITHOUT fix

Ensure the TCPServer fix is not applied:

```bash
# Make sure fix is stashed
git stash list  # Should show your fix

# Run stress test
cd pkg/network/tracer
./run_stress_test.sh fast
```

**Expected result**: Should see failures like:
```
FAILURE #1 on test 42: write failed (local=127.0.0.1:52568, remote=127.0.0.1:46631): 
write tcp 127.0.0.1:52568->127.0.0.1:46631: write: bad file descriptor
```

### Step 2: Verify fix resolves the issue

Apply the fix and retest:

```bash
# Apply your fix
git stash pop

# Run stress test again
./run_stress_test.sh full  # Use full test for high confidence
```

**Expected result**: All tests pass
```
=== Stress Test Results ===
Total tests: 10000
Passed: 10000 (100.00%)
Failed: 0 (0.00%)
```

## Understanding the Output

### Progress Updates
```
Progress: 1000/10000 completed (10.0%), 2 failures, 125 tests/sec, ~1m12s remaining
```

### Success (No failures)
```
=== Stress Test Results ===
Total tests: 10000
Passed: 10000 (100.00%)
Failed: 0 (0.00%)
Duration: 1m23.4s
Rate: 120 tests/second

All 10000 tests passed! (Expected ~200 failures based on CI rate)
```

### Failure Detected
```
FAILURE #1 on test 342: write failed (local=127.0.0.1:52568, remote=127.0.0.1:46631): 
write tcp 127.0.0.1:52568->127.0.0.1:46631: write: bad file descriptor

=== Stress Test Results ===
Total tests: 10000
Passed: 9823 (98.23%)
Failed: 177 (1.77%)

Stress test detected 177 failures out of 10000 runs (1.77% failure rate)
```

## Troubleshooting

### Tests pass even without fix

If stress tests pass without the fix, try:

1. **Increase parallelism**:
   ```bash
   go test -tags test -v -run TestTCPRTTStressParallel -parallel=100 -timeout=15m
   ```

2. **Reduce delays** in the test to trigger race more aggressively:
   - Edit `stress_test_tcprtt.go`
   - Change `time.Sleep(5 * time.Millisecond)` to `time.Sleep(0)`

3. **Run under system stress**:
   ```bash
   # Start CPU stress in background
   stress-ng --cpu 8 --timeout 60s &
   
   # Run test
   ./run_stress_test.sh full
   ```

4. **Use Go race detector** (slower but more sensitive):
   ```bash
   go test -tags test -race -v -run TestTCPRTTStressFast -timeout=10m
   ```

### Tests are too slow

For faster iteration:
- Use `TestTCPRTTStressFast` (1K iterations)
- Reduce sleep times in the test
- Lower parallel worker count

## Cleanup

These are temporary test files. After validating your fix:

```bash
rm stress_test_tcprtt.go
rm run_stress_test.sh  
rm STRESS_TEST_README.md
rm stress_test_output.log  # If created
```

## CI Integration

**`TestTCPRTTStressCI` WILL run in CI** as part of the normal test suite. It:
- Runs 200 iterations (~30-60 seconds)
- Has 98% chance of catching the 2% failure rate
- Will FAIL if the TCPServer fix is not applied
- Will PASS once the fix is in place

The other stress tests (`TestTCPRTTStressFast`, `TestTCPRTTStressParallel`, `TestTCPRTTStress`) skip in short mode and are for local validation only.

## See Also

- Original flaky test: `tracer_linux_test.go::TestTCPRTT`
- TCPServer implementation: `testutil/tcp.go`
- Fix: Add `sync.WaitGroup`, connection tracking, and proper shutdown to `TCPServer`

