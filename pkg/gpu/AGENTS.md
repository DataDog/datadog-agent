# GPU Package - Architecture Guide for AI Assistants

> **Note to AI Assistants**: Keep this file updated when you discover new patterns,
> fix bugs, or learn something significant about the codebase. This helps future
> assistants avoid repeating mistakes and understand the architecture faster.

## Overview

The GPU package monitors CUDA applications by intercepting kernel launches, memory allocations, and synchronization events via eBPF. Events are processed into spans that get aggregated into metrics.

## Package Structure

```
pkg/gpu/
├── probe.go           # Main entry point, sets up eBPF uprobes
├── consumer.go        # Reads events from eBPF ringbuffer
├── stream.go          # StreamHandler processes events per CUDA stream
├── stream_collection.go # Manages all stream handlers
├── stats.go           # statsGenerator creates metrics from stream data
├── aggregator.go      # Aggregates data from multiple streams per process
├── context.go         # systemContext holds GPU device info, caches
├── cuda/              # CUDA binary parsing (fatbin, cubin, symbols)
├── safenvml/          # Safe wrapper around NVML library
├── ebpf/              # eBPF program and types
├── containers/        # Container detection for GPU workloads
└── config/            # Configuration
```

## Data Flow

```
1. Probe (probe.go)
   └── Sets up eBPF uprobes on CUDA library functions (cudaLaunchKernel, cudaMalloc, etc.)

2. eBPF Program (ebpf/c/runtime/gpu.c)
   └── Intercepts calls, sends events via ringbuffer

3. Consumer (consumer.go)
   └── Reads from ringbuffer, routes to appropriate StreamHandler

4. StreamHandler (stream.go)
   └── Processes events, generates spans on sync events

5. statsGenerator (stats.go)
   └── Collects spans from all streams, distributes to aggregators

6. Aggregator (aggregator.go)
   └── Combines data from multiple streams into process-level stats

7. Probe.GetAndFlush()
   └── Returns final metrics to the core check
```

## Key Components

### StreamHandler (`stream.go`)

Handles events for a single CUDA stream. Key fields:

- `kernelLaunches []*enrichedKernelLaunch` - Pending kernel launches awaiting sync
- `pendingKernelSpans chan *kernelSpan` - Finalized kernel spans awaiting collection
- `pendingMemorySpans chan *memorySpan` - Finalized memory spans awaiting collection
- `memAllocEvents *lru.LRU` - Active memory allocations (waiting for free)
- `ended bool` - Marks handler as ended, should not receive new events

### StreamCollection (`stream_collection.go`)

Manages all stream handlers. Uses `sync.Map` for thread-safe access:
- `streams` - Non-global streams (keyed by pid + stream_id)
- `globalStreams` - Global streams (keyed by pid + gpu_uuid)

### Memory Pools (`stream.go`)

Three pools for frequently allocated objects to reduce GC pressure:

```go
type memoryPools struct {
    enrichedKernelLaunchPool  // Kernel launch events
    kernelSpanPool            // Finalized kernel spans
    memorySpanPool            // Finalized memory spans
}
```

**Critical**: Every `Get()` must have a corresponding `Put()` or you'll leak memory.

## Event Flow

```
eBPF Event → Consumer → getStream() → StreamHandler.handle*()
                                            ↓
                                    kernelLaunches (pool)
                                            ↓
                            markSynchronization() on sync event
                                            ↓
                                    pendingKernelSpans (pool)
                                            ↓
                                    getPastData() → metrics
                                            ↓
                                    releaseSpans() → Put back to pool
```

## Pool Object Lifecycle

### enrichedKernelLaunch
1. `Get()` in `handleKernelLaunch()`
2. Stored in `kernelLaunches` slice
3. `Put()` in `markSynchronization()` after processing

### kernelSpan
1. `Get()` in `getCurrentKernelSpan()`
2. Sent to `pendingKernelSpans` channel (or `Put()` if no kernels match)
3. Consumed via `getPastData()`
4. `Put()` via `streamSpans.releaseSpans()`

### memorySpan
1. `Get()` in `handleMemEvent()` or `getAssociatedAllocations()`
2. Sent to `pendingMemorySpans` channel
3. Consumed via `getPastData()`
4. `Put()` via `streamSpans.releaseSpans()`

## Handler Cleanup

Handlers are cleaned in two scenarios:

### 1. Process Exit
- `markProcessStreamsAsEnded()` → `markEnd()` → sets `ended = true`
- Emits final spans for pending data
- Next cleanup cycle removes from map

### 2. Inactivity Timeout
- `cleanHandlerMap()` detects inactive handlers
- **Delete from map first** (prevents new events)
- Then `releasePoolResources()` (releases all pool objects silently)

### releasePoolResources()

Releases all held resources without emitting spans:
- Kernel launches → back to pool
- Pending kernel spans → drained from channel, back to pool
- Pending memory spans → drained from channel, back to pool

**Important**: Delete handler from map BEFORE calling this to prevent race conditions where new events are added during cleanup.

## Common Pitfalls

### Pool Leaks
- Every `Get()` needs a `Put()` on all code paths
- When filtering (e.g., `getCurrentKernelSpan` returns nil), must `Put()` back
- When channels are full, `trySendSpan` handles the `Put()` automatically

### Race Conditions
- Stream handlers can be accessed concurrently (consumer thread vs cleanup)
- Use `kernelLaunchesMutex` for `kernelLaunches` slice access
- Delete from map before cleanup to prevent new references

### Channel Draining
- When draining channels during cleanup, limit iterations to prevent blocking
- Use `cap(channel)` as upper bound for iterations

## Testing Pools

Use `withTelemetryEnabledPools(t, telemetryMock)` to enable pool telemetry in tests.

Check pool stats with:
```go
stats := getPoolStats(t, telemetryMock, "enrichedKernelLaunch")
require.Equal(t, 0, stats.active)  // No leaked items
require.Equal(t, stats.get, stats.put)  // Balanced get/put
```

## Thread Safety

- **Consumer thread**: Calls `getStream()`, `handle*()` methods
- **Stats/cleanup thread**: Calls `clean()`, `getPastData()`
- `sync.Map` provides thread-safe map access
- `kernelLaunchesMutex` protects the launches slice and `ended` flag

---

## Other Components (Light Overview)

### Probe (`probe.go`)
- Main entry point, implements `system-probe` module interface
- Sets up eBPF manager with uprobes on CUDA library functions
- Uses `sharedlibraries` to attach to dynamically loaded libcuda
- `GetAndFlush()` returns metrics to the GPU core check

### Consumer (`consumer.go`)
- Runs in dedicated goroutine reading from eBPF ringbuffer
- Routes events to appropriate StreamHandler via StreamCollection
- Handles process exit notifications from process monitor

### statsGenerator (`stats.go`)
- Called by `Probe.GetAndFlush()` to generate metrics
- Iterates all streams, collects past and current data
- Distributes to per-process aggregators
- Returns `model.GPUStats` for the core check

### Aggregator (`aggregator.go`)
- One per process, combines data from all streams
- Generates process-level GPU utilization metrics
- Tracks memory usage across allocations
- Computes `ActiveTimePct` by merging kernel execution intervals

### systemContext (`context.go`)
- Holds GPU device info (via NVML)
- Caches: visible devices per process, selected device per thread
- `cudaKernelCache` for parsed CUDA binary info

### CUDA Parsing (`cuda/`)
- Parses fatbin/cubin embedded in CUDA binaries
- Extracts kernel metadata (size, shared memory, etc.)
- `KernelCache` loads kernel info asynchronously in background

### safenvml (`safenvml/`)
- Safe wrapper around NVIDIA NVML library
- Handles library loading, error recovery
- Caches device info to reduce NVML calls

---

## Active Time Metrics (`sm_active` and `process.sm_active`)

### Overview

The GPU monitoring system emits two metrics related to GPU active time:
- **`process.sm_active`**: Per-process percentage of time the GPU had active kernels for that process
- **`sm_active`**: Device-wide percentage of time the GPU had any active kernels

Both metrics are emitted with **Low priority** to act as fallbacks when NVML GPM-based metrics are unavailable.

### Calculation

#### Per-Process Active Time (`process.sm_active`)

1. **Interval Tracking**: For each kernel span, the aggregator records the time interval `[startKtime, endKtime]`
2. **Interval Merging**: Overlapping intervals within the same process are merged using `mergeIntervals()`
3. **Percentage Calculation**: `ActiveTimePct = (mergedDuration / intervalNs) * 100`

#### Device-Level Active Time (`sm_active`)

1. **Collect All Intervals**: The `statsGenerator` collects kernel span intervals from all processes on a device
2. **Merge Across Processes**: Intervals are merged to handle overlapping execution across processes
3. **Percentage Calculation**: Same as per-process, but using the merged intervals from all processes

### Implementation Details

#### `mergeIntervals()` Function (`aggregator.go`)

```go
func mergeIntervals(intervals [][2]uint64) uint64
```

- Takes unsorted time intervals `[start, end]`
- Sorts by start time
- Merges overlapping/adjacent intervals
- Returns total duration covered
- Time complexity: O(n log n)

**Example**:
```
Input:  [[1, 5], [3, 8], [10, 15]]
Merged: [[1, 8], [10, 15]]
Total:  7 + 5 = 12 nanoseconds
```

#### Boundary Clamping

Kernel span times are clamped to the stats generation interval:
```go
if lastGenerationKTime > start {
    start = lastGenerationKTime
}
if end > nowKtime {
    end = nowKtime
}
```

This ensures we only count activity within the current measurement window.

### Data Flow

```
1. StreamHandler.processKernelSpan()
   └── Records [startKtime, endKtime] in aggregator.activeIntervals

2. Aggregator.getRawStats()
   └── Merges intervals, computes per-process ActiveTimePct

3. statsGenerator.getStats()
   ├── Collects all intervals per device
   ├── Merges intervals across processes
   └── Computes device-level ActiveTimePct

4. ebpfCollector.Collect()
   ├── Emits process.sm_active (per-process, Low priority)
   └── Emits sm_active (device-wide, Low priority)
```

### Metric Priority

Both metrics use **Low priority** to ensure NVML GPM-based `sm_active` metrics (which have Medium priority) take precedence when available. This makes the eBPF-based metrics act as fallbacks.

### Testing

Run tests using the appropriate command based on what you're testing:

#### System-Probe Tests (for `pkg/gpu/*` - eBPF/system-probe code)
```bash
dda inv -e system-probe.test --packages=./pkg/gpu
```

This runs tests for the system-probe GPU module (probe.go, stats.go, aggregator.go, stream.go, etc.).

#### Agent Tests (for `pkg/collector/corechecks/gpu/*` - agent-side code)
```bash
dda inv test --targets=./pkg/collector/corechecks/gpu/nvidia --build-include=nvml,test
```

This runs tests for agent-side collectors (ebpf.go, process collector, etc.). The `test` build tag is required to include test dependencies.

**Key distinction**:
- Use `system-probe.test` for eBPF/system-probe packages
- Use `test` for agent core check packages

#### Test Coverage

- **Unit tests** (`aggregator_test.go`): Test `mergeIntervals()` function with various scenarios (empty, overlapping, adjacent, unsorted, edge cases)
- **Integration tests** (`stats_test.go`): Test per-process and device-level `ActiveTimePct` calculation with overlapping spans
- **End-to-end tests** (`probe_test.go`): Verify metrics are computed from real CUDA samples
- **Collector tests** (`ebpf_test.go`): Verify metrics are emitted with correct priority

### Common Pitfalls

1. **Overlapping Intervals**: Don't sum durations directly; use `mergeIntervals()` to handle overlaps
2. **Boundary Clamping**: Always clamp span times to the measurement interval
3. **Percentage Cap**: Cap at 100% to handle edge cases where spans extend beyond the interval
4. **Priority**: Ensure Low priority is set so NVML metrics take precedence
