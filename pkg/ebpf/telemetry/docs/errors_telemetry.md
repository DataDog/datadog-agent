# EBPF errors telemetry
EBPF errors telemetry collection refers to collecting errors code for failed helper operations. We collect to categories of failures:
- Map update failures
- Helper failures. These include
    - `bpf_perf_event_output`
    - `bpf_probe_read_*`
    - `bpf_skb_load_bytes`

For maps, errors are collected per map per error code.
For helpers, errors are collected per probe per helper per error code.

## Implementation
This telemetry collection mechanism has two parts
1. Telemetry instrumentation
2. Telemetry macros

### 1. Telemetry instrumentation
Telemetry instrumentation is a form of [eBPF instrumentation](./ebpf_instrumentation.md) responsible for getting a pointer to the map value. This map value is the structure for holding the telemetry data.
Once the map value is acquired the instrumentation code caches it on the stack, so that it may be used in the telemetry macros for recording error telemetry.

#### Pointer Caching
When building eBPF bytecode, we pass the options [-stack-size-section](https://github.com/llvm-mirror/llvm/blob/master/include/llvm/CodeGen/CommandFlags.inc#L268) to `llc`. This instructs it to record stack usage information for each function in dedicated sections. At load-time we parse these sections
and calculate if there is a free 8 byte slot available for a function. If so, we use this to cache the pointer value.
If no slot is available then telemetry collection for this program is not supported.

### 2. Telemetry macros
These macros are defined in [pkg/ebpf/c/bpf_telemetry.h](../../c/bpf_telemetry.h). The macros are basically responsible for collecting the return value of the helper operation and saving it.
If a helper operation fails, then the macros read the cached value pointing to the telemetry struct and record the failure.
