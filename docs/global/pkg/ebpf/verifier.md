# pkg/ebpf/verifier

## Purpose

Provides tooling to load every eBPF program in a set of object files through the kernel verifier and collect the complexity statistics it reports. The results can be serialized to JSON for CI quality gates, dashboards, or offline analysis. An optional `--line-complexity` mode also maps verifier-log instructions back to C source lines using `llvm-objdump`.

## Key elements

### Build flags

| Build tag | File | Notes |
|-----------|------|-------|
| `linux_bpf` | `stats.go`, `elf.go`, `verifier_log_parser.go` | All analysis logic requires Linux + BPF. |
| (no build tag) | `stats_no_linux.go` | Stub so the package compiles on non-Linux. |
| (no build tag) | `types.go` | Type definitions compile everywhere. |

Minimum kernel for statistics collection: **4.15** (checked at runtime by `BuildVerifierStats`).

### Types

| Type | Description |
|------|-------------|
| `Statistics` | Per-program verifier statistics. Fields are tagged with the minimum kernel version that exposes them. |
| `stat` | Internal carrier for a single parsed value and its regex. |
| `StatsOptions` | Input to `BuildVerifierStats`: list of `.o` files, optional program-name regex filters, `DetailedComplexity` flag, and an optional directory to persist raw verifier logs. |
| `StatsResult` | Output of `BuildVerifierStats`: `Stats map[string]*Statistics`, `Complexity map[string]*ComplexityInfo`, and `FuncsPerSection map[string]map[string][]string`. Keys use `"<object>/<program>"` format. |
| `ComplexityInfo` | Per-program instruction map and source-line map. Only populated when `DetailedComplexity` is true. |
| `InstructionInfo` | Per-instruction data: index, times processed by the verifier, BPF assembly code, register state, and back-reference to a `SourceLine`. |
| `SourceLineStats` | Aggregate complexity for a single C source line: instruction count, max/min pass counts, total instructions processed. |
| `SourceLine` | C source line text and DWARF line-info annotation. |
| `RegisterState` | State of a single BPF register as reported by the verifier log. |

### Statistics fields and kernel requirements

| Field | JSON key | Min kernel |
|-------|----------|-----------|
| `StackDepth` | `stack_usage` | 4.15 |
| `InstructionsProcessed` | `instruction_processed` | 4.15 |
| `InstructionsProcessedLimit` | `limit` | 4.15 |
| `MaxStatesPerInstruction` | `max_states_per_insn` | 5.2 |
| `TotalStates` | `total_states` | 5.2 |
| `PeakStates` | `peak_states` | 5.2 |

### Key functions

| Function | Description |
|----------|-------------|
| `BuildVerifierStats(opts *StatsOptions) (*StatsResult, map[string]struct{}, error)` | Main entry point. Iterates over the provided `.o` files, loads each program individually with `LogLevelStats` (and `LogLevelInstruction` when `DetailedComplexity` is set), parses the verifier log, and collects results. The second return value is the set of programs that failed to load. CO-RE assets (files inside a `co-re/` subdirectory) are loaded via `ddebpf.LoadCOREAsset`; non-CO-RE assets are opened directly. |

### CO-RE vs prebuilt detection

`isCOREAsset(path)` returns true when the file's parent directory is named `co-re`. This matches the layout produced by the agent's build system where CO-RE objects sit in `$BPF_DIR/co-re/<name>.o`.

### Memory management note

With `DetailedComplexity` enabled the kernel allocates up to 1 GB for the verifier log per program. After each program the package explicitly calls `debug.FreeOSMemory()` to return memory to the OS and avoid OOM kills in CI environments with restricted memory.

### `calculator` sub-command

`pkg/ebpf/verifier/calculator/main.go` is a standalone binary that wraps `BuildVerifierStats`. It is the primary consumer of this package. It:

1. Reads the eBPF object directory from `$DD_SYSTEM_PROBE_BPF_DIR`.
2. Prefers CO-RE objects when both variants exist.
3. Copies files to a temp directory owned by root (required by `VerifyAssetPermissions`).
4. Writes `summary.json` with per-program `Statistics`.
5. Optionally writes per-program `ComplexityInfo` JSON files under a `complexity-data/` tree.

Run it with:
```
DD_SYSTEM_PROBE_BPF_DIR=/opt/datadog-agent/embedded/share/system-probe/ebpf \
  ./bin/ebpf-verifier-calculator \
  --summary-output=/tmp/summary.json \
  [--line-complexity] \
  [--filter-file=conntrack.o] \
  [--filter-prog=kprobe__tcp_connect]
```

### Linux-specific requirements

- Must run as root (or with `CAP_BPF` + `CAP_SYS_ADMIN`) so the kernel verifier runs.
- `rlimit.RemoveMemlock()` must be called before `BuildVerifierStats` (handled in `calculator/main.go`).
- `llvm-objdump` must be on `PATH` when `DetailedComplexity` is true.

## Usage

The package is consumed almost exclusively by the `calculator` binary (CI pipeline). Direct programmatic use follows this pattern:

```go
opts := &verifier.StatsOptions{
    ObjectFiles:        []string{"/tmp/tracer.o", "/tmp/co-re/dns.o"},
    FilterPrograms:     []*regexp.Regexp{regexp.MustCompile("kprobe__")},
    DetailedComplexity: false,
    VerifierLogsDir:    "/tmp/vlog",
}

results, failed, err := verifier.BuildVerifierStats(opts)
if err != nil { /* handle */ }

for prog, stat := range results.Stats {
    fmt.Printf("%s: %d instructions processed\n", prog, stat.InstructionsProcessed.Value)
}
```

The only production caller outside the `calculator` binary is `pkg/ebpf/verifier/calculator/main.go` itself.
