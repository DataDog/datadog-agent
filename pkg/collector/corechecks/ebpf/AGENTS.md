# eBPF Core Checks

Note: This guide covers simple, single-purpose eBPF checks under `pkg/collector/corechecks/ebpf/` ("container integration" checks). For complex/standalone features (e.g., network, GPU, dynamic instrumentation), see their dedicated packages and docs; refer to `.cursor/rules/system_probe_modules.mdc` for pointers.

## Structure

Each eBPF-based check consists of three components:

See also: `.cursor/rules/system_probe_modules.mdc` for system-probe module context and cross-links.

1. **Probe** (`probe/<check>/`) - System-probe side eBPF implementation
   - `<check>.go` - Tracer with eBPF map management, NewTracer(), GetAndFlush(), Close()
   - `<check>_kern_types.go` - CGO bridge for C structs (build tag: `//go:build ignore`)
   - `model/*.go` - API data models (exported types for stats)
   - `<check>_stub.go` - Stub implementation for non-linux_bpf platforms
   - Build tags: `//go:build linux_bpf` for main implementation

2. **Check** (`<check>/`) - Agent side metric collection
   - `<check>.go` - Check implementation with system-probe client
   - Communicates via HTTP with system-probe module
   - Converts probe stats to Datadog metrics
   - Build tags: `//go:build linux`
   - `stub.go` - Stub for non-linux platforms

3. **System-Probe Module** (`cmd/system-probe/modules/<check>.go`)
   - Module registration and HTTP endpoint
   - Wraps the probe tracer
   - Build tags: `//go:build linux && linux_bpf`

## eBPF C Code

Located in `c/runtime/<check>-kern.c`:
- Per-CPU maps for lock-free stats collection
- Helper functions from `cgroup.h`, `bpf_helpers.h`, `bpf_tracing.h`
- CO-RE (Compile Once Run Everywhere) macros for portability
- Shared header `<check>-kern-user.h` defines C structs

## Implementation Pattern

### 1. Define Data Model (`probe/<check>/model/`)
```go
type StatsKey struct { /* fields */ }
type StatsValue struct { /* fields */ }
type Stats map[StatsKey]StatsValue
```

### 2. Create eBPF Program (`c/runtime/<check>-kern.c`)
- Define BPF maps (usually per-CPU hash maps)
- Implement kprobes/kretprobes/tracepoints
- Update counters in maps

### 3. Create Tracer (`probe/<check>/<check>.go`)
- `NewTracer(cfg *ebpf.Config)` - Initialize with CO-RE + runtime fallback
- `GetAndFlush()` - Iterate maps, aggregate per-CPU data, delete entries
- `Close()` - Cleanup resources

### 4. Create System-Probe Module (`cmd/system-probe/modules/`)
- Register module with factory
- Expose `/check` HTTP endpoint
- Track last check time for health monitoring

### 5. Create Agent Check (`<check>/<check>.go`)
- Factory function accepting tagger component
- Configure() - Setup system-probe client
- Run() - Fetch stats, extract container IDs, tag metrics, submit

### 6. Register Check (`pkg/commonchecks/corechecks_sysprobe.go`)
```go
corecheckLoader.RegisterCheck(<check>.CheckName, <check>.Factory(tagger))
```

### 7. Add Configuration (`pkg/config/setup/system_probe.go`)
```go
const <check>NS = "<check_name>"
cfg.BindEnvAndSetDefault(join(<check>NS, "enabled"), false)
```

### 8. Add Static Config Listener (`comp/core/autodiscovery/listeners/staticconfig.go`)
```go
if enabled := pkgconfigsetup.SystemProbe().GetBool("<check_name>.enabled"); enabled {
    l.newService <- &StaticConfigService{adIdentifier: "_<check_name>"}
}
```

### 9. Register eBPF Programs in Build System (`tasks/system_probe.py`)

**For container integration checks** (like oom-kill, tcp-queue-length, seccomp-tracer):

In `ninja_container_integrations_ebpf_programs()`, add your program name to the list:
```python
container_integrations_co_re_programs = ["oom-kill", "tcp-queue-length", "ebpf", "seccomp-tracer"]
```
- File must be named `<check>-kern.c` in `pkg/collector/corechecks/ebpf/c/runtime/`
- Automatically builds CO-RE and debug versions

**Add runtime compilation support** in `ninja_runtime_compilation_files()`:
```python
runtime_compiler_files = {
    # ... existing entries
    "pkg/collector/corechecks/ebpf/probe/<check>/<check>.go": "<check-name>",
}
```
- This enables runtime compilation fallback when CO-RE isn't available
- The key is the Go file with `//go:generate` directives
- The value is the base name for generated C and Go files

**Add CGO type generation** in `ninja_cgo_type_files()`:
```python
def_files = {
    # ... existing entries
    "pkg/collector/corechecks/ebpf/probe/<check>/<check>_kern_types.go": [
        "pkg/collector/corechecks/ebpf/c/runtime/<check>-kern-user.h",
    ],
}
```
- Generates Go types from C structs for BPF map keys/values
- Header file must use `__u32`, `__u64` etc. types and include `ktypes.h`

**For other eBPF program types:**
- Network programs: Add to `ninja_network_ebpf_programs()`
- GPU programs: Add to `ninja_gpu_ebpf_programs()`
- Discovery programs: Add to `ninja_discovery_ebpf_programs()`
- Dynamic instrumentation: Add to `ninja_dynamic_instrumentation_ebpf_programs()`

## Building

- Runtime compilation: Requires kernel headers
- CO-RE: Pre-compiled with BTF, fallback to runtime compilation
- Build: `dda inv system-probe.build --build-include linux_bpf`

## Testing

- Test file: `probe/<check>/<check>_test.go` with build tag `//go:build linux_bpf`
- Use `ebpftest.TestBuildModes` to test both CO-RE and runtime-compiled modes
- Create sample C program in `testdata/` to trigger the monitored events
- Tests should verify:
  1. Probe loads successfully
  2. Events are captured correctly
  3. Stats are aggregated properly

Example test structure:
```go
type checkTestSuite struct { suite.Suite }

func TestCheck(t *testing.T) {
    ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.CORE, ebpftest.RuntimeCompiled}, "",
        func(t *testing.T) {
            suite.Run(t, new(checkTestSuite))
        })
}
```

## Examples

- **tcp_queue_length**: Monitors TCP queue usage per container
- **oom_kill**: Detects OOM kill events
- **seccomp_tracer**: Tracks seccomp denial events

## Best Practices

- Use per-CPU maps to avoid lock contention
- Always aggregate per-CPU data in GetAndFlush()
- Clean up maps after reading to prevent memory leaks
- Use cgroup helpers to extract container IDs
- Wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Tag metrics with container ID and other relevant dimensions
- Provide CO-RE + runtime compilation fallback
- Test both build modes
