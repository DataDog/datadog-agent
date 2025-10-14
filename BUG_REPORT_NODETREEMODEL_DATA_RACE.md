# Data Race Bug Report: pkg/config/nodetreemodel

## Summary

A data race exists in `pkg/config/nodetreemodel` where concurrent `GetBool()` calls (and other getter methods) can result in one goroutine writing to `c.root` while another reads from it, with both goroutines holding only read locks (`RLock`).

## Impact

- **Severity**: High - data race can cause undefined behavior
- **Affected tests**: Any test using `configmock.New(t)` with concurrent config access
- **First observed**: CI failure in `pkg/serverless/metrics::TestRaceFlushVersusParsePacket`
- **Introduced in**: Commit `05e00bf3dd1da372b71d9d422056d09c43c572c4` (March 28, 2025)

## Root Cause

The bug is in the interaction between these functions:

1. `GetBool()` (and similar getters) acquire a **read lock** (`RLock`)
2. They call `checkKnownKey()`
3. `checkKnownKey()` calls `maybeRebuild()`
4. `maybeRebuild()` calls `buildSchema()` when `allowDynamicSchema` is true
5. `buildSchema()` calls `mergeAllLayers()`
6. `mergeAllLayers()` **WRITES** to `c.root` at line 483

**The problem**: Step 6 performs a write while the caller in step 1 holds only a read lock. Other concurrent readers also hold read locks, creating a race condition.

### Code Flow

```go
// pkg/config/nodetreemodel/getter.go:210-218
func (c *ntmConfig) GetBool(key string) bool {
    c.RLock()              // ← Only a READ lock!
    defer c.RUnlock()
    c.checkKnownKey(key)   // ← This can trigger WRITES
    b, err := cast.ToBoolE(c.getNodeValue(key))
    // ...
}

// pkg/config/nodetreemodel/config.go:444-445
func (c *ntmConfig) checkKnownKey(key string) {
    c.maybeRebuild()  // ← Called under read lock
    // ...
}

// pkg/config/nodetreemodel/config.go:430-437
func (c *ntmConfig) maybeRebuild() {
    if c.allowDynamicSchema.Load() {
        c.allowDynamicSchema.Store(false)
        defer func() { c.allowDynamicSchema.Store(true) }()
        c.buildSchema()  // ← Calls mergeAllLayers() which writes to c.root
    }
}

// pkg/config/nodetreemodel/config.go:460-487
func (c *ntmConfig) mergeAllLayers() error {
    // ... merge logic ...
    c.root = merged  // ← LINE 483: WRITE while caller holds only READ lock!
    // ...
}
```

## Reproduction

### Prerequisites
- Go 1.24+ with race detector
- Datadog Agent codebase at commit `05e00bf3dd1` or later

### Steps to Reproduce

1. Run the minimal reproduction test:
   ```bash
   cd /path/to/datadog-agent
   go test -race -run TestConcurrentGetBoolDataRace ./pkg/config/nodetreemodel -v -count=1
   ```

2. The race detector will report:
   ```
   WARNING: DATA RACE
   Write at 0x... by goroutine X:
     github.com/DataDog/datadog-agent/pkg/config/nodetreemodel.(*ntmConfig).mergeAllLayers()
         config.go:483
     github.com/DataDog/datadog-agent/pkg/config/nodetreemodel.(*ntmConfig).buildSchema()
         config.go:536
     github.com/DataDog/datadog-agent/pkg/config/nodetreemodel.(*ntmConfig).maybeRebuild()
         config.go:436
     github.com/DataDog/datadog-agent/pkg/config/nodetreemodel.(*ntmConfig).checkKnownKey()
         config.go:445
     github.com/DataDog/datadog-agent/pkg/config/nodetreemodel.(*ntmConfig).GetBool()
         getter.go:213

   Previous read at 0x... by goroutine Y:
     github.com/DataDog/datadog-agent/pkg/config/nodetreemodel.(*ntmConfig).getNodeValue()
         getter.go:141
     github.com/DataDog/datadog-agent/pkg/config/nodetreemodel.(*ntmConfig).GetBool()
         getter.go:214
   ```

### Test Location

The minimal reproduction test is located at:
- `pkg/config/nodetreemodel/race_test.go::TestConcurrentGetBoolDataRace`

This test reproduces the exact race condition seen in production CI failures.

## When Does This Occur?

The race manifests when:

1. **Dynamic schema is enabled**: `SetTestOnlyDynamicSchema(true)` is called
   - This happens in ALL tests using `configmock.New(t)`
   - See `pkg/config/mock/mock.go:56`

2. **Concurrent config access**: Multiple goroutines call `GetBool/GetInt/GetString` simultaneously
   - Common in tests with demultiplexers, aggregators, serializers
   - Example: `pkg/serverless/metrics/metric_test.go::TestRaceFlushVersusParsePacket`

## Original CI Failure Example

From `tests_nodetreemodel` CI job:

```
WARNING: DATA RACE
Write at 0x00c00247e2b0 by goroutine 5520:
  pkg/config/nodetreemodel.(*ntmConfig).mergeAllLayers()
      config.go:483
  ...
  pkg/serializer.(*Serializer).SendIterableSeries()
      serializer.go:227

Previous read at 0x00c00247e2b0 by goroutine 4680:
  pkg/config/nodetreemodel.(*ntmConfig).getNodeValue()
      getter.go:141
  ...
  pkg/aggregator.(*TimeSampler).sendTelemetry()
      time_sampler.go:304
```

## Proposed Fixes

### Option 1: Lock Upgrade in maybeRebuild() (Quick Fix)

```go
func (c *ntmConfig) maybeRebuild() {
    if c.allowDynamicSchema.Load() {
        // Upgrade from read lock to write lock
        c.RUnlock()
        c.Lock()
        defer c.Unlock()
        defer c.RLock()  // Re-acquire read lock before returning

        // Double-check condition after acquiring write lock
        if !c.allowDynamicSchema.Load() {
            return
        }

        c.allowDynamicSchema.Store(false)
        defer func() { c.allowDynamicSchema.Store(true) }()
        c.buildSchema()
    }
}
```

**Pros**: Minimal change, fixes the immediate issue
**Cons**: Lock upgrade pattern is tricky and can be error-prone

### Option 2: Remove maybeRebuild from Read Path (Better Fix)

Don't call `maybeRebuild()` from functions holding read locks. Instead:
- Only call `buildSchema()` from write operations (which hold write locks)
- Rethink the `SetTestOnlyDynamicSchema` feature to not require rebuilding during reads

**Pros**: Cleaner separation of read/write operations
**Cons**: Requires more extensive refactoring

### Option 3: Use atomic.Pointer for c.root

Replace `c.root` with `atomic.Pointer[InnerNode]` for lock-free reads:

```go
type ntmConfig struct {
    root atomic.Pointer[InnerNode]
    // ...
}
```

**Pros**: Lock-free reads, no contention
**Cons**: Requires changes throughout codebase where `c.root` is accessed

## Related Information

- **Commit that introduced bug**: `05e00bf3dd1da372b71d9d422056d09c43c572c4`
- **PR**: #35462 "Prepare fixes for tests when nodetreemodel is used by default"
- **Date**: March 28, 2025
- **Author**: Dustin Long

## Files Affected

- `pkg/config/nodetreemodel/config.go:430-487` - Contains the buggy maybeRebuild/mergeAllLayers
- `pkg/config/nodetreemodel/getter.go:210-218` - GetBool and other getters call checkKnownKey
- `pkg/config/mock/mock.go:56` - Enables SetTestOnlyDynamicSchema in all test configs

## Additional Notes

- This bug ONLY affects tests (SetTestOnlyDynamicSchema is test-only)
- Production code is not affected (dynamic schema is not enabled in production)
- However, the bug indicates a design issue that should be fixed
- The race detector correctly identifies this as a real concurrency bug
