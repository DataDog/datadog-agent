> **TL;DR:** `pkg/dyninst` implements dynamic instrumentation (Live Debugger) for Go processes by compiling Remote Config probe definitions into eBPF stack-machine programs that are attached as uprobes to capture variable values at arbitrary locations without restarting the target.

# pkg/dyninst

## Purpose

`pkg/dyninst` implements **dynamic instrumentation** (also called Live Debugger) for Go
processes. It attaches eBPF uprobes to running Go binaries at locations specified by remote
probe configurations, captures local variable values and expression results at those locations,
and delivers structured snapshots to the Datadog backend — all without restarting the target
process or modifying its source code.

The package requires Linux and the `linux_bpf` build tag. It is exposed to the rest of the
agent as a system-probe module (`pkg/dyninst/module`).

### High-level data flow

```
Remote Config (RC)
      |
      v
   rcjson.Probe  ──► irgen.Generator ──► ir.Program
                                              |
                               compiler.GenerateCode
                                              |
                              loader (eBPF load + uprobe attach)
                                              |
                              output.Event (ring buffer read)
                                              |
                               uploader → Datadog intake
```

The `actuator` package is the central state machine that drives this pipeline in response to
process lifecycle events and RC updates.

## Key elements

### Configuration and build flags

All implementation files require the `linux_bpf` build tag. The `ir` and `rcjson` sub-packages are exceptions — they compile without it for use in tooling and tests. Configuration for circuit-breaker CPU limits, rate limits, and type-discovery caps is supplied via `actuator.Config`.

### Key types

#### Sub-packages

| Package | Role |
|---------|------|
| `rcjson` | Deserializes probe definitions received from Remote Configuration into Go structs. Entry point: `UnmarshalProbe([]byte)`. |
| `ir` | Intermediate representation: describes a set of probes as they apply to a single binary. Central type: `ir.Program`. |
| `irgen` | Generates an `ir.Program` from an ELF object file and a list of probe definitions by parsing DWARF debug info. Entry point: `irgen.Generator.GenerateIR`. |
| `compiler` | Compiles an `ir.Program` into a platform-independent stack-machine bytecode program. Entry point: `compiler.GenerateCode`. |
| `ebpf` | C eBPF source and supporting header files for the uprobe stack machine. Also contains the Go framing helpers used to read ring-buffer output. |
| `uprobe` | Attaches compiled programs to processes via Linux uprobes. |
| `output` | Parses raw ring-buffer events emitted by the eBPF program into `output.Event` values that the output pipeline can interpret. |
| `actuator` | Primary stateful component. Manages the lifecycle (load → attach → detach → unload) of eBPF programs for each process. |
| `loader` | Loads the compiled eBPF bytecode and uprobe attachment points into the kernel. |
| `module` | System-probe module entrypoint (`module.Module`). Wires together all sub-packages. |
| `dispatcher` | Routes RC probe updates and process events to the actuator. |
| `procsubscribe` | Subscribes to process start/stop events from the kernel. |
| `uploader` | Buffers and forwards captured snapshots to the Datadog intake. |
| `exprlang` | AST definition for the Datadog expression language (DSL) used in probe conditions and templates. |
| `irprinter` | Serializes an `ir.Program` to JSON for debugging. |

#### Core types

#### `ir` package

```go
// Program: everything needed to generate and interpret an eBPF program
// for a single binary.
type Program struct {
    ID           ProgramID
    Probes       []*Probe
    Subprograms  []*Subprogram   // functions instrumented by the probes
    Types        map[TypeID]Type // Go types referenced by variables/expressions
    Issues       []ProbeIssue    // probes that could not be compiled
    GoModuledataInfo GoModuledataInfo
    CommonTypes  CommonTypes
}

// ProbeDefinition: common interface implemented by all probe kinds.
type ProbeDefinition interface {
    ProbeIDer
    GetKind() ProbeKind     // Log, Snapshot, Metric, Span, CaptureExpression
    GetWhere() Where        // FunctionWhere or LineWhere
    GetCaptureConfig() CaptureConfig
    GetThrottleConfig() ThrottleConfig
    GetTemplate() TemplateDefinition
    GetWhen() json.RawMessage   // conditional expression (nil = unconditional)
    GetWhenDSL() string
    GetCaptureExpressions() []CaptureExpressionDefinition
}

// Type: interface for Go types embedded in the IR.
type Type interface {
    GetID() TypeID
    GetName() string
    GetByteSize() uint32
    GetGoKind() (reflect.Kind, bool)
    // ... (concrete subtypes: StructureType, SliceType, MapType, etc.)
}
```

`VariableRole` constants: `VariableRoleParameter`, `VariableRoleReturn`, `VariableRoleLocal`.

`ProbeKind` constants: `ProbeKindLog`, `ProbeKindSnapshot`, `ProbeKindMetric`,
`ProbeKindSpan`, `ProbeKindCaptureExpression`.

#### `rcjson` package

Concrete types that implement `ir.ProbeDefinition` and map 1-to-1 to the JSON payloads from
the Remote Configuration control plane:

| Type | Description |
|------|-------------|
| `LogProbe` | Emits a structured log message using a template. |
| `SnapshotProbe` | Captures a full variable snapshot (`captureSnapshot=true`). |
| `CaptureExpressionProbe` | Captures a named set of DSL expressions. |
| `MetricProbe` | Emits a metric (count / gauge / histogram). |
| `SpanProbe` | Decorates the active APM span. |

Entry point:

```go
probe, err := rcjson.UnmarshalProbe(rawJSON)
if err := rcjson.Validate(probe); err != nil { ... }
```

`ProbeCommon` embeds the shared fields (`ID`, `Version`, `Where`, `Tags`, `Language`,
`EvaluateAt`).

#### `irgen` package

```go
type Generator struct { /* config */ }

func NewGenerator(options ...Option) *Generator

// GenerateIR parses the ELF/DWARF info from obj, resolves the probe locations,
// and returns an ir.Program ready for compilation.
func (g *Generator) GenerateIR(
    obj object.File,
    programID ir.ProgramID,
    probes []ir.ProbeDefinition,
    additionalTypes []string,
) (*ir.Program, error)
```

#### `actuator` package

```go
type Actuator struct { /* internal state machine */ }

func NewActuator(cfg Config) *Actuator
func (a *Actuator) SetRuntime(runtime Runtime)

// HandleUpdate is the primary external API: call it when probe or process
// configurations change.
func (a *Actuator) HandleUpdate(update ProcessesUpdate)

// ReportMissingTypes feeds back type names observed at runtime (interface
// decoding) so the actuator can trigger recompilation.
func (a *Actuator) ReportMissingTypes(processID ProcessID, typeNames []string)

func (a *Actuator) Shutdown() error
func (a *Actuator) Stats() map[string]any
func (a *Actuator) DebugInfo() *DebugInfo
```

`Config` fields of note:
- `CircuitBreakerConfig` — CPU limits enforced per probe and across all probes.
- `DiscoveredTypesLimit` — cap on the number of runtime-discovered type names
  retained across services.
- `RecompilationRateLimit` / `RecompilationRateBurst` — rate limits for IR
  recompilation triggered by missing types.

The `Runtime` interface (implemented by `loader`) is injected via `SetRuntime`:

```go
type Runtime interface {
    Load(ir.ProgramID, Executable, ProcessID, []ir.ProbeDefinition, LoadOptions) (LoadedProgram, error)
}

type LoadedProgram interface {
    Attach(ProcessID, Executable) (AttachedProgram, error)
    RuntimeStats() []loader.RuntimeStats
    Close() error
}
```

#### `compiler` package

```go
// GenerateCode compiles an ir-level Program into a bytecode sequence
// and feeds it to the provided CodeSerializer.
func GenerateCode(program Program, out CodeSerializer) (CodeMetadata, error)

type CodeSerializer interface {
    CommentBlock(comment string) error
    SerializeInstruction(opcode Opcode, paramBytes []byte, comment string) error
    // ...
}
```

#### `output` package

```go
// DataItem: a single typed value read from the eBPF ring buffer.
type DataItem struct { /* header + raw bytes */ }

func (d *DataItem) Type() uint32
func (d *DataItem) IsFailedRead() bool
func (d *DataItem) Data() ([]byte, bool)
```

`EventPairingExpectation` constants (e.g. `EventPairingExpectationEntryPairingExpected`,
`EventPairingExpectationConditionFailed`) control how the output pipeline pairs entry and
return events.

### Key interfaces

#### eBPF stack machine

The eBPF C code lives in `pkg/dyninst/ebpf/`. It implements a custom stack machine
(`stack_machine.h`) executed inside the uprobe handler. The machine reads variable values
from the target process's memory and stacks them into a per-CPU scratch buffer before writing
the framed event to a ring buffer.

## Usage

### Within the agent

`pkg/dyninst/module` is the system-probe module that wires everything together. It is
registered and loaded by the system-probe binary when dynamic instrumentation is enabled.

```go
// Typical initialization sequence (simplified from module/module.go):
m, err := module.NewModule(config, rcSubscriber)
// The module starts background goroutines for process subscription,
// IR generation, and eBPF loading. Probe updates arrive through the
// Remote Config subscriber (ProductLiveDebugging / "LIVE_DEBUGGING"),
// which routes them to the actuator via the dispatcher.
```

The eBPF program is loaded using the same CO-RE / runtime-compilation / prebuilt fallback
chain defined in `pkg/ebpf` (see [`pkg/ebpf`](ebpf.md) for the `COREResult`/`BTFResult` status
codes and the three-step fallback chain). Internally, `loader` calls `ebpf.LoadCOREAsset` and
wraps the resulting manager with `ebpf.NewManagerWithDefault` (including
`telemetry.ErrorsTelemetryModifier`). Uprobe attachment is delegated to
`pkg/ebpf/uprobes.UprobeAttacher` configured with `AttachToExecutable` rules for each Go
binary that has active probes. Unlike the USM TLS probes (which use
`AttachToSharedLibraries`), dyninst attaches exclusively to Go executables and generates a
unique eBPF program per probe at runtime — see [`pkg/ebpf/uprobes`](ebpf/uprobes.md) for the
`AttachRule`/`AttachTarget` API and the `uprobe__<symbol>` naming convention.

ELF/DWARF parsing in `irgen` uses `pkg/util/safeelf` exclusively (see
[`pkg/util/safeelf`](util/safeelf.md)). This means `irgen.Generator.GenerateIR` will return
an error (never panic) when given a malformed binary. `safeelf.Open` is the only safe way to
read ELF files in this codebase (enforced by the `depguard` linter); any direct use of
`debug/elf` in a new sub-package of `pkg/dyninst` must be replaced with `safeelf`.

### Comparison with the SECL compiler

`pkg/dyninst` and [`pkg/security/secl/compiler`](security/secl-compiler.md) both compile a
user-supplied expression language into an evaluation engine, but they solve different
problems:

| Aspect | `pkg/dyninst` | `pkg/security/secl/compiler` |
|--------|--------------|-------------------------------|
| Input | JSON probe definition from Remote Config | SECL rule string from policy files |
| Target | Custom eBPF stack-machine bytecode (`compiler.GenerateCode`) | Go closure (`eval.RuleEvaluator`) |
| Execution | In-kernel, inside a uprobe handler | In-process, inside the CWS event loop |
| Runtime type resolution | Yes — `actuator.ReportMissingTypes` triggers recompilation | No — field types are static in the model |
| Probe lifecycle | Managed by `actuator` (load → attach → detach → unload) | Managed by the rules engine |

Both packages parse an expression language into an IR and compile it to an evaluator, but
dyninst's evaluator is an eBPF program loaded into the kernel whereas SECL's is a pure Go
closure. Neither package depends on the other.

### Lifecycle of a probe

1. Remote Config pushes a JSON probe definition.
2. `rcjson.UnmarshalProbe` deserializes it into a concrete `Probe` type.
3. The dispatcher calls `actuator.HandleUpdate` with the new probe set.
4. The actuator calls `irgen.Generator.GenerateIR` for each affected process binary.
5. The resulting `ir.Program` is compiled to bytecode by `compiler.GenerateCode`.
6. The bytecode is loaded into the kernel via `loader` and attached to the process uprobes.
7. When the uprobe fires, the eBPF stack machine captures variable values and writes them
   to the ring buffer.
8. `output` reads and decodes ring-buffer events; `uploader` forwards snapshots to intake.

### Testing

End-to-end integration tests live in `pkg/dyninst/end_to_end_test.go` and
`pkg/dyninst/integration_test.go`. They use test programs from `pkg/dyninst/testprogs/` to
exercise the full pipeline from probe definition to captured snapshot.

Run with:

```bash
dda inv test --targets=./pkg/dyninst/... --build-include=linux_bpf
```

## Related packages

- `cmd/system-probe/modules/` — registers `pkg/dyninst/module` as a system-probe module.
- `pkg/flare/archive_linux.go` — includes dyninst diagnostics in flares.
- `pkg/dyninst/exprlang` — expression language AST shared between `rcjson` and `irgen`.
- [`pkg/ebpf`](ebpf.md) — shared eBPF infrastructure consumed by `pkg/dyninst`. The `pkg/ebpf`
  `Manager` and its loading strategies (CO-RE / runtime compilation / prebuilt) are the
  mechanism used by `loader` to load the eBPF stack-machine program. See the loading strategy
  section in `ebpf.md` for the fallback chain and `COREResult` telemetry.
- [`pkg/ebpf/uprobes`](ebpf/uprobes.md) — `UprobeAttacher` is used internally by
  `pkg/dyninst/uprobe` to attach compiled programs to Go processes. The same attacher is also
  used by `pkg/gpu` and the USM TLS probes; see `uprobes.md` for the `AttachRule` /
  `AttachTarget` API and the probe naming convention (`uprobe__<symbol>`).
- [`pkg/util/safeelf`](util/safeelf.md) — `pkg/dyninst/irgen` calls `safeelf.Open` / `f.DWARF()`
  to parse ELF/DWARF debug info from target binaries without risking a panic on malformed ELF
  files. `safeelf` is the canonical ELF import for the whole codebase (enforced by `depguard`).
- [`pkg/remoteconfig/state`](remoteconfig.md) — probe definitions reach dyninst through the
  Remote Configuration system. The `ProductLiveDebugging` RC product (`LIVE_DEBUGGING` string
  value) carries the JSON probe payloads deserialized by `rcjson.UnmarshalProbe`. The
  `dispatcher` subscribes to RC updates and routes them to the `actuator` after status
  acknowledgement via `state.ApplyStatus`. For details on how RC products are registered and
  how apply-status acknowledgement flows back to the backend, see the
  [`pkg/remoteconfig/state`](remoteconfig.md) reference, specifically the `Repository.Update`
  / `UpdateApplyStatus` API and the `ProductLiveDebugging` constant in `products.go`.
- [`pkg/security/secl/compiler`](security/secl-compiler.md) — a structurally similar
  expression-to-evaluator compilation pipeline used by the CWS (Cloud Workload Security)
  subsystem. Both packages compile a user-supplied expression language into an evaluation
  engine, but SECL compiles to Go closures evaluated in-process while dyninst compiles to
  eBPF stack-machine bytecode executed in-kernel. See the "Comparison with the SECL compiler"
  section above for a side-by-side summary.
