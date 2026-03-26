> **TL;DR:** Converts kernel monotonic timestamps from eBPF events into absolute wall-clock `time.Time` values by tracking boot time and applying drift-corrected offsets on Linux.

# pkg/util/ktime

## Purpose

`pkg/util/ktime` converts kernel monotonic timestamps (nanoseconds since boot, as reported by eBPF programs via `bpf_ktime_get_ns()`) to absolute `time.Time` wall-clock values. This translation is necessary because eBPF events carry only a monotonic counter; to correlate them with real-world times, the agent must know the system boot time and the current offset between the monotonic clock and wall time.

The package is Linux-only (build tag `linux`).

## Key elements

### `Resolver` (`resolver.go`)

```go
type Resolver struct {
    bootTime time.Time
}
```

| Function / Method | Description |
|---|---|
| `NewResolver() (*Resolver, error)` | Creates a `Resolver` by reading the system boot time via `gopsutil/host.BootTime()`. Returns an error if boot time cannot be determined. |
| `(*Resolver).ResolveMonotonicTimestamp(timestamp uint64) time.Time` | Converts a kernel `uint64` nanosecond monotonic timestamp to `time.Time`. Returns the zero `time.Time` for a zero input. |
| `(*Resolver).ApplyBootTime(timestamp time.Time) time.Time` | Adjusts a `time.Time` that was recorded relative to boot (e.g. from a `/proc` field) to an absolute wall-clock time. |
| `(*Resolver).ComputeMonotonicTimestamp(timestamp time.Time) int64` | Inverse of `ResolveMonotonicTimestamp`: converts an absolute `time.Time` back to a monotonic nanosecond offset. |
| `(*Resolver).GetBootTime() time.Time` | Returns the boot time, adjusted by the current uptime offset for drift correction. |

#### Clock drift correction

The resolver accounts for accumulated drift between the monotonic clock and wall time by computing `getUptimeOffset()` at each call:

```
offset = time.Since(bootTime) - runtime.nanotime()
```

`nanotime()` is linked directly to the Go runtime's internal monotonic clock via `//go:linkname`, which is approximately 2x faster than `time.Now()`. This makes timestamp resolution suitable for high-frequency event processing.

## Usage

`pkg/util/ktime` is used wherever the agent processes events from eBPF probes that carry monotonic timestamps:

- `pkg/security/resolvers/process/resolver_ebpf.go` — resolves process event timestamps for the Cloud Workload Security (CWS) probe.
- `pkg/security/resolvers/resolvers_ebpf.go` — creates a shared `Resolver` for all CWS resolvers.
- `pkg/network/tracer/tracer.go` — converts network event timestamps from the USM/NPM eBPF tracer.

Typical usage:

```go
resolver, err := ktime.NewResolver()
if err != nil {
    return err
}
// timestamp is a uint64 from an eBPF ring buffer event
wallTime := resolver.ResolveMonotonicTimestamp(event.Timestamp)
```
