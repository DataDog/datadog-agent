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

### 9. Register eBPF Programs in Build System

eBPF programs, CGO type generation, and runtime compilation bundles are all
managed by **Bazel**.

**Add the eBPF CO-RE program** in the check's `c/runtime/BUILD.bazel` using
`ebpf_program_suite` (see existing targets in
`pkg/collector/corechecks/ebpf/c/runtime/BUILD.bazel`). Then:
1. Add the target to `_BAZEL_EBPF_CORE_TARGETS` in `tasks/system_probe.py`
   (needed for the copy step that stages `.o` files).
2. Add it to the `all_ebpf_programs` filegroup in `pkg/ebpf/BUILD.bazel`.

**Add runtime compilation support** by creating a `runtime_compilation_bundle`
target in `pkg/ebpf/bytecode/BUILD.bazel`:
```python
runtime_compilation_bundle(
    name = "<check-name>",
    header_deps = _CORECHECK_HEADERS,
    include_dirs = ["pkg/ebpf/c"],
    out_go_file = "//pkg/ebpf/bytecode/runtime:<check-name>.go",
    out_name = "<check-name>",
    src_c = "//pkg/collector/corechecks/ebpf/c/runtime:<check-name>-kern.c",
)
```
Then add the `_flat` target to `_BAZEL_RUNTIME_FLAT_TARGETS` in
`tasks/system_probe.py` and both the `_flat` and `_verify_test` targets to the
convenience targets in `pkg/ebpf/BUILD.bazel` (`all_ebpf_programs` and
`verify_generated_files` respectively).

**Add CGO type generation** by creating a `cgo_godefs` target in the check's
`BUILD.bazel`:
```python
load("//bazel/rules/ebpf:cgo_godefs.bzl", "cgo_godefs")

exports_files(["<check>_kern_types.go", "<check>_kern_types_linux.go", "<check>_kern_types_linux_test.go"])

cgo_godefs(
    name = "<check>_kern_types_godefs",
    src = "<check>_kern_types.go",
)
```
Then add the `_test` and `_test_file_test` targets to the
`verify_generated_files` test suite in `pkg/ebpf/BUILD.bazel`.

- Generates Go types from C structs for BPF map keys/values
- Header file must use `__u32`, `__u64` etc. types and include `ktypes.h`
- `bazel test //pkg/ebpf:verify_generated_files` checks all committed files
- `bazel run //<pkg>:<name>_godefs` regenerates a single output

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
