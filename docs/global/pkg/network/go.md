# pkg/network/go

**Base path:** `github.com/DataDog/datadog-agent/pkg/network/go`
**Team:** ebpf-platform / universal-service-monitoring

## Purpose

`pkg/network/go` is a collection of sub-packages for inspecting compiled Go binaries at runtime. The Network Performance Monitoring (NPM) and Universal Service Monitoring (USM) features use it to attach eBPF uprobes to TLS functions inside Go processes (e.g. `crypto/tls.(*Conn).Write/Read/Close`) without requiring any instrumentation in the target binary.

Inspection happens when the agent detects a new Go process and needs to know:
- where in the binary a given function starts (for uprobe attachment),
- which registers or stack slots hold function arguments (for parameter extraction in the eBPF probe),
- the offset of `goid` in `runtime.g` (to correlate events across goroutines).

The sub-packages are layered; callers normally use `bininspect` as the top-level API and only reach into the lower packages for specialized needs.

---

## Sub-packages

### `bininspect` — binary inspection

**Build constraint:** `linux`

The main entry point for binary inspection. Parses an ELF file and returns a `Result` containing everything an eBPF program needs to hook a Go binary.

#### Key types

| Type | Description |
|---|---|
| `Result` | Top-level output: `Arch`, `ABI`, `GoVersion`, `Functions` map, `StructOffsets` map, `GoroutineIDMetadata` |
| `FunctionMetadata` | `EntryLocation` (uprobe attach address/offset), `Parameters []ParameterMetadata`, `ReturnLocations []uint64` |
| `ParameterMetadata` | Total size, `reflect.Kind`, and `Pieces []ParameterPiece` describing where each piece lives (stack or register) |
| `ParameterPiece` | A word-sized sub-piece of a parameter: `InReg bool`, `Register int`, or `StackOffset int64` |
| `FieldIdentifier` | `{StructName, FieldName}` pair used as a map key for struct field offsets |
| `GoroutineIDMetadata` | Offset of `goid` in `runtime.g`, and whether `runtime.g` pointer is in a register (`RuntimeGRegister`) or TLS (`RuntimeGTLSAddrOffset`) |
| `GoArch` | `"amd64"` or `"arm64"` |
| `GoABI` | `"stack"` (pre-Go 1.17 register ABI) or `"register"` |
| `FunctionConfiguration` | Controls whether return locations are collected and provides a `ParameterLookupFunction` |
| `StructLookupFunction` | Version/arch-keyed lookup for struct field offsets (used by `InspectNewProcessBinary`) |

#### Pre-defined struct field identifiers

The package exports named `FieldIdentifier` variables for all fields used by the TLS uprobe:

`StructOffsetTLSConn`, `StructOffsetTCPConn`, `StructOffsetNetConnFd`, `StructOffsetFamilyInNetFD`, `StructOffsetLaddrInNetFD`, `StructOffsetRaddrInNetFD`, `StructOffsetPortInTCPAddr`, `StructOffsetIPInTCPAddr`, `StructOffsetLimitListenerConnNetConn`

#### TLS function constants

```go
WriteGoTLSFunc = "crypto/tls.(*Conn).Write"
ReadGoTLSFunc  = "crypto/tls.(*Conn).Read"
CloseGoTLSFunc = "crypto/tls.(*Conn).Close"
```

#### Key functions

| Function | Description |
|---|---|
| `InspectNewProcessBinary(elf, functions, structs)` | Production path. Reads function entry/return locations and parameter layouts from the ELF symbol table and version-keyed lookup tables. Does not require DWARF symbols in the target binary. |
| `InspectWithDWARF(elf, dwarfData, functions, structFields)` | Test/tooling path. Uses DWARF debug info for richer metadata (exact parameter locations from debug info entries). Requires a non-stripped binary. |
| `GetAllSymbolsByName(elf, filter)` | Extracts a filtered set of ELF symbols from `.symtab` or `.dynsym`. Custom filter types (`stringSetSymbolFilter`, `infixSymbolFilter`) allow efficient scanning without loading all symbols. |
| `GetAnySymbolWithInfix / GetAnySymbolWithInfixPCLNTAB` | Locate a symbol matching a name infix (used to detect Go runtime symbols). |
| `FindReturnLocations(elf, sym, offset)` | Delegates to `asmscan.ScanFunction` to find all `RET` instructions in a function's machine code. |
| `FindGoVersion(elf)` | Reads the Go version string embedded in the binary. |
| `FindABI(version, arch)` | Returns `GoABIRegister` for Go ≥ 1.17 on amd64, `GoABIStack` otherwise. |

---

### `dwarfutils` — DWARF utilities

Helpers for navigating DWARF debug information in Go ELF binaries.

#### `TypeFinder`

```go
type TypeFinder struct { ... }
func NewTypeFinder(dwarfData *dwarf.Data) *TypeFinder
```

Wraps `github.com/go-delve/delve/pkg/dwarf/godwarf` with a type cache and fixes two upstream bugs:
- `godwarf` returns `reflect.Invalid` for slice and interface types; `TypeFinder` corrects these to `reflect.Slice` and `reflect.Interface`.
- Recurses through `typedef` chains before applying the fix.

Key methods:
- `FindTypeByName(name)` — scans DWARF entries for a type with a matching name attribute.
- `FindTypeByOffset(offset)` — reads and caches the type at a given DWARF offset.
- `FindStructFieldOffset(structName, fieldName)` — combines the above to return the byte offset of a named field within a named struct or slice type.

#### `compile_unit.go`

`LoadCompileUnits` / `FindCompileUnit(pc)` — loads all DWARF compile-unit entries and finds the one containing a given program counter. Used when resolving location lists that require a base address.

#### `entry.go`

`GetChildLeafEntries(reader, parentOffset, tag)` — collects all leaf DWARF entries with a given tag that are direct children of a parent entry. Used by `bininspect` to enumerate formal-parameter entries.

#### `locexpr` — DWARF location expression executor

`locexpr.Exec(expression, totalSize, pointerSize)` statically evaluates a DWARF location expression (DWARF v4 §2.5/2.6/7.7) and returns a `[]LocationPiece` describing whether each piece of a value lives in a register or on the stack.

Key design choices:
- Injects sentinel values for CFA and frame base to detect when an offset was derived from them, then subtracts the sentinel to recover the actual stack offset relative to SP.
- Deduplicates identical pieces (observed in some compiler outputs).
- Returns an empty slice for empty location expressions (optimized-away values).

---

### `goversion` — Go version wrapper

```go
type GoVersion struct {
    goversion.GoVersion  // from go-delve/delve
    rawVersion string
}
func NewGoVersion(rawVersion string) (GoVersion, error)
func (v *GoVersion) AfterOrEqual(other GoVersion) bool
func (v GoVersion) String() string
```

Wraps delve's `GoVersion` to preserve the raw version string. This disambiguates `"1.19"` (no patch) from `"1.19.0"` — delve normalizes both to the same struct, losing the distinction needed by some lookup tables.

---

### `goid` — goroutine ID offset lookup table

Provides a pre-generated lookup table (`goid_offset.go`) for the byte offset of the `goid` field within `runtime.g` for every supported Go version and architecture.

```go
// Supported: amd64, arm64. Minimum: Go 1.13.
func GetGoroutineIDOffset(version goversion.GoVersion, goarch string) (uint64, error)
```

The table is generated by `go generate` via `internal/generate_goid_lut.go`, which downloads each Go toolchain, compiles a test program that prints the offset at runtime, and records the result. The generated file is checked in so no toolchain downloads are needed at agent build time.

`bininspect.InspectNewProcessBinary` calls this function to populate `GoroutineIDMetadata.GoroutineIDOffset`.

---

### `asmscan` — machine code scanner

Provides `ScanFunction` and architecture-specific instruction scanners.

```go
func ScanFunction(
    textSection *safeelf.Section,
    sym safeelf.Symbol,
    functionOffset uint64,
    scanInstructions func(data []byte) ([]uint64, error),
) ([]uint64, error)
```

Reads the machine code bytes for one function from the ELF `.text` section and delegates to `scanInstructions`, which returns offsets relative to the buffer. `ScanFunction` then adjusts these to absolute virtual addresses (or file offsets for PIE binaries).

Pre-built scanner callbacks:

| Function | Architecture | Notes |
|---|---|---|
| `FindX86_64ReturnInstructions(data)` | x86-64 | Uses `golang.org/x/arch/x86/x86asm`; decodes one instruction at a time, advances cursor by `Len` |
| `FindARM64ReturnInstructions(data)` | ARM64 | Uses `golang.org/x/arch/arm64/arm64asm`; instructions are always 4 bytes |

**Why scan for return instructions?** Go's `uretprobe` (kernel mechanism for hooking function exits) does not work correctly with Go binaries because Go's goroutine stack-switching can violate the assumptions `uretprobe` makes. The workaround is to attach a regular uprobe to every `RET` instruction in the function body.

---

### Supporting packages

| Package | Purpose |
|---|---|
| `binversion` | Reads the Go build info embedded in a binary (`debug/buildinfo`) to extract the Go version string used by `FindGoVersion`. |
| `lutgen` | Code-generation helpers for producing version/arch lookup-table Go source files. Used by `goid/internal/generate_goid_lut.go` and by `pkg/network/protocols/http/gotls/lookup`. |
| `rungo` | Downloads and caches specific Go toolchain versions; used only at code-generation time. |

---

## Architecture: how the sub-packages layer

```
bininspect                  ← top-level API used by callers
  ├── dwarfutils/           ← DWARF navigation (test/tooling path only)
  │     ├── locexpr/        ← DWARF location expression executor
  │     └── compile_unit.go, entry.go
  ├── asmscan/              ← machine-code scanner (RET instruction finder)
  ├── goid/                 ← goroutine-ID offset lookup table
  └── goversion/            ← Go version wrapper

Supporting (code-generation time only):
  ├── lutgen/               ← LUT source-file generator
  └── rungo/                ← downloads specific Go toolchains
```

The split between `InspectNewProcessBinary` (production, no DWARF required) and `InspectWithDWARF` (test/tooling) is intentional: stripped Go binaries in production containers usually have no DWARF, so the production path relies entirely on the symbol table and pre-generated lookup tables.

---

## Usage

### Hooking a new Go TLS process (production path)

```go
import (
    "github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
    "github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

elfFile, _ := safeelf.Open(binaryPath)
defer elfFile.Close()

result, err := bininspect.InspectNewProcessBinary(
    elfFile,
    map[string]bininspect.FunctionConfiguration{
        bininspect.WriteGoTLSFunc: {
            IncludeReturnLocations: true,
            ParamLookupFunction:    myLutLookup,
        },
    },
    map[bininspect.FieldIdentifier]bininspect.StructLookupFunction{
        bininspect.StructOffsetTLSConn: myStructLut,
    },
)
// result.Functions[WriteGoTLSFunc].EntryLocation -> uprobe attach address
// result.Functions[WriteGoTLSFunc].ReturnLocations -> uretprobe workaround addresses
// result.StructOffsets[StructOffsetTLSConn] -> field byte offset for eBPF map
```

### Callers in the codebase

- `pkg/network/usm/ebpf_gotls.go` — main consumer; calls `InspectNewProcessBinary` for each new Go process detected by the USM eBPF program.
- `pkg/network/usm/ebpf_gotls_helpers.go` — helpers for uprobe attachment using `bininspect.Result`.
- `pkg/network/protocols/http/gotls/lookup/` — generates and consumes version-keyed parameter lookup tables (LUTs) for TLS function arguments using `lutgen`.
- `pkg/network/protocols/tls/gotls/` — TLS-specific uprobe definitions that consume `bininspect.WriteGoTLSFunc`, `ReadGoTLSFunc`, and `CloseGoTLSFunc` constants.

### End-to-end flow: Go TLS uprobe attachment

```
usm.Monitor.Start()
    |
    v
sharedlibraries.EbpfProgram detects a Go binary via execve / do_sys_open hook
    |
    v
ebpf_gotls.go: new Go process event arrives
    |
    v
safeelf.Open(binaryPath)
bininspect.InspectNewProcessBinary(elf, functions, structs)
    +--> bininspect.FindGoVersion()          via binversion (reads build info)
    +--> bininspect.FindABI()                register vs stack ABI (Go ≥ 1.17)
    +--> goid.GetGoroutineIDOffset()         from pre-generated LUT
    +--> asmscan.ScanFunction() per func     find all RET instructions
    |
    v
Result.Functions[WriteGoTLSFunc].EntryLocation
Result.Functions[WriteGoTLSFunc].ReturnLocations  (one uprobe per RET)
    |
    v
pkg/ebpf/uprobes.UprobeAttacher.AttachToExecutable()
```

The `ReturnLocations` list is necessary because `uretprobe` is unreliable for Go binaries (goroutine stack-copying can move the stack between entry and return). `asmscan.FindX86_64ReturnInstructions` / `FindARM64ReturnInstructions` scan the machine code to locate every `RET` instruction instead.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/network/usm` | [usm.md](usm.md) | Primary consumer of `bininspect`. `ebpf_gotls.go` calls `InspectNewProcessBinary` for each newly detected Go process, then uses the returned `Result` to attach uprobes via `UprobeAttacher`. |
| `pkg/network/protocols` | [protocols.md](protocols.md) | `protocols/tls/gotls/` defines the Go TLS uprobe probe selectors and uses the `WriteGoTLSFunc`, `ReadGoTLSFunc`, and `CloseGoTLSFunc` constants exported by `bininspect`. `protocols/http/gotls/lookup/` uses `lutgen` to generate version-keyed parameter LUTs consumed by `InspectNewProcessBinary`. |
| `pkg/ebpf/uprobes` | [../../pkg/ebpf/uprobes.md](../../pkg/ebpf/uprobes.md) | `UprobeAttacher` is the mechanism that actually attaches the probes once `bininspect` has resolved the addresses. `ProbeOptions.IsManualReturn` is set for Go functions so that `UprobeAttacher` attaches to every address in `ReturnLocations` rather than using a `uretprobe`. |
