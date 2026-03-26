> **TL;DR:** Provides panic-safe wrappers around Go's standard ELF parser and re-exports all ELF constants, serving as the canonical and linter-enforced import point for ELF inspection across the agent's eBPF and profiling subsystems.

# pkg/util/safeelf

## Purpose

`pkg/util/safeelf` provides panic-safe wrappers around Go's standard `debug/elf` package. The standard library's ELF parser is known to panic on certain malformed or unusual binaries (see the [open Go issues](https://github.com/golang/go/issues?q=is%3Aissue+is%3Aopen+debug%2Felf+in%3Atitle)). This package catches those panics and converts them into ordinary errors, preventing agent crashes when inspecting arbitrary binaries on a host.

It is also the canonical import point for ELF constants and types used across the codebase: all consumers import `safeelf` instead of `debug/elf` directly (enforced by the `depguard` linter rule).

## Key elements

### `File` (`elf.go`)

```go
type File struct {
    *elf.File
}
```

Thin wrapper around `*elf.File`. Inherits all `elf.File` fields (sections, programs, machine, class, etc.) and overrides the methods most likely to panic.

| Function / Method | Description |
|---|---|
| `NewFile(r io.ReaderAt) (*File, error)` | Parse an ELF from any `io.ReaderAt`. Recovers panics. |
| `Open(path string) (*File, error)` | Parse an ELF from a path. `Close()` also closes the underlying `os.File`. Recovers panics. |
| `(*File).Symbols() ([]Symbol, error)` | Safe wrapper — recovers panics from the static symbol table. |
| `(*File).DynamicSymbols() ([]Symbol, error)` | Safe wrapper — recovers panics from the dynamic symbol table (`.dynsym`). |
| `(*File).DWARF() (*dwarf.Data, error)` | Safe wrapper — recovers panics from DWARF debug info parsing. |
| `(*File).SectionsByType(typ SectionType) []*elf.Section` | Returns all sections matching the given `SectionType`. Convenience helper absent from the standard library. |

### Re-exported types and constants (`types.go`)

`types.go` re-exports the full set of ELF types and constants used by the agent so that callers never need to import `debug/elf` themselves:

- Types: `Prog`, `Symbol`, `SymType`, `SymBind`, `Section`, `SectionHeader`, `SectionType`, `SectionIndex`, `SectionFlag`, `Machine`
- Section-type constants: `SHT_SYMTAB`, `SHT_DYNSYM`, `SHT_NOTE`, `SHT_REL`, `SHT_RELA`, `SHT_HASH`, `SHT_DYNAMIC`, `SHT_GNU_HASH`, `SHT_GNU_VERDEF`, `SHT_GNU_VERNEED`, `SHT_GNU_VERSYM`, `SHT_NOBITS`, `SHT_PROGBITS`
- Section-flag constants: `SHF_ALLOC`, `SHF_EXECINSTR`, `SHF_WRITE`, `SHF_COMPRESSED`
- Symbol-binding/type constants: `STB_GLOBAL`, `STB_WEAK`, `STT_OBJECT`, `STT_FUNC`, `STT_FILE`
- Special section indices: `SHN_UNDEF`, `SHN_ABS`, `SHN_COMMON`
- ELF class constants: `ELFCLASS32`, `ELFCLASS64`, `ET_EXEC`, `ET_DYN`
- Architecture constants: `EM_X86_64`, `EM_AARCH64`
- Program header constants: `PT_LOAD`, `PT_TLS`, `PF_X`, `PF_W`, `PF_R`
- Symbol size constants: `Sym32Size`, `Sym64Size`
- Helper functions: `ST_TYPE(info uint8) SymType`, `ST_BIND(info uint8) SymBind`
- Error sentinel: `ErrNoSymbols`

## Usage

`safeelf` is used throughout the eBPF and profiling subsystems where agent code inspects binaries at runtime:

- `pkg/dyninst/irgen` — calls `safeelf.Open` then `f.DWARF()` to parse DWARF debug info from target Go binaries for dynamic instrumentation. Because `irgen` may encounter arbitrary user binaries, panic recovery is essential here.
- `pkg/network/usm/utils/file_registry.go` — calls `safeelf.NewFile(f)` to inspect shared libraries (e.g. `libssl`, `libcrypto`) and checks `ErrNoSymbols` when the dynamic symbol table is absent.
- `pkg/network/usm/ebpf_gotls_helpers.go` — calls `safeelf.NewFile` to read Go TLS function offsets (entry/return addresses) from a Go binary before attaching uprobes; handles `ErrNoSymbols` for stripped binaries.
- `comp/host-profiler/symboluploader/` — parses ELF symbol tables and pclntab sections to upload symbols for Go and native profiling.

Typical usage pattern:

```go
f, err := safeelf.Open("/path/to/binary")
if err != nil {
    // handles both open errors and panics-turned-errors
    return err
}
defer f.Close()

syms, err := f.Symbols()
if errors.Is(err, safeelf.ErrNoSymbols) {
    // binary was stripped; fall back to dynamic symbols
    syms, err = f.DynamicSymbols()
}
```

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/dyninst` | [../dyninst.md](../dyninst.md) | `irgen.Generator.GenerateIR` opens target binaries exclusively via `safeelf.Open` / `f.DWARF()`. The panic-recovery guarantee means `GenerateIR` returns an error rather than crashing the system-probe process on malformed ELF files. `safeelf` is the only ELF import allowed in `irgen` (enforced by `depguard`). |
| `pkg/network/usm` | [../network/usm.md](../network/usm.md) | `utils/file_registry.go` and `ebpf_gotls_helpers.go` use `safeelf` to inspect shared libraries detected by `sharedlibraries.EbpfProgram` and to locate Go TLS function offsets before `UprobeAttacher` attaches probes. `ErrNoSymbols` is the expected sentinel for stripped production binaries. |
| `pkg/ebpf` | [../ebpf.md](../ebpf.md) | `pkg/ebpf` loads compiled eBPF object files (`.o`) as ELF; that path uses `cilium/ebpf` directly rather than `safeelf`. `safeelf` is reserved for inspecting *user-space* binaries at runtime (uprobe targets), not eBPF object files. |
