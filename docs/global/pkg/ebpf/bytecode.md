> **TL;DR:** `pkg/ebpf/bytecode` abstracts eBPF object file delivery behind a single `AssetReader` interface, supporting both pre-compiled files bundled in the binary (bindata) and on-demand compilation from embedded C source via `clang`.

# pkg/ebpf/bytecode

## Purpose

Provides the plumbing to load compiled eBPF object files into the agent at runtime. It abstracts two delivery modes—files bundled inside the binary at build time (bindata) and files read from disk—behind a single `AssetReader` interface. The `runtime/` sub-package adds a third mode: on-the-fly compilation from C source via `clang`.

## Key elements

### Key interfaces

| Type | Description |
|------|-------------|
| `AssetReader` | `asset_reader.go` — combines `io.Reader`, `io.ReaderAt`, and `io.Closer`. All callers that load eBPF objects expect this interface; all three loading strategies (bindata, disk, runtime-compiled) satisfy it. |

### Key types

| Type | Location | Description |
|------|----------|-------------|
| `AssetReader` | `asset_reader.go` | Combines `io.Reader`, `io.ReaderAt`, and `io.Closer`. All callers that load eBPF objects expect this interface. |
| `asset` | `runtime/asset.go` | Internal struct that holds a C source filename and its expected SHA-256 hash. Drives the compile-then-verify flow. |
| `CompiledOutput` | `runtime/runtime_compilation_helpers.go` | `io.Reader + io.ReaderAt + io.Closer` returned after runtime compilation. Identical contract to `AssetReader`. |
| `CompileOptions` | `runtime/asset.go` | Options passed to the compiler: extra `clang` flags, an optional content-modification callback, and whether to resolve kernel headers. |
| `ProtectedFile` | `runtime/protected_file.go` | A RAM-backed or disk-backed immutable copy of the C source, used to guarantee integrity before compilation starts. |

### Key functions

| Function | Description |
|----------|-------------|
| `GetReader(dir, name string) (AssetReader, error)` | Entry point for callers. Under `ebpf_bindata` build tag reads from the embedded `bindata` blob; otherwise opens the file from `dir` on disk after checking ownership/permissions. |
| `VerifyAssetPermissions(path string) error` | (`permissions.go`) Ensures the `.o` file is owned by root, preventing tampering. Called unconditionally on disk-mode paths. |
| `asset.Compile(config, flags)` / `asset.CompileWithOptions(config, opts)` | Triggers runtime compilation: locates kernel headers, creates a `ProtectedFile`, optionally runs a `ModifyCallback`, then invokes `clang` via `pkg/ebpf/compiler`. Result is cached on disk; subsequent calls reuse the cached `.o` if the uname + source hash + flag hash are unchanged. |

### Configuration and build flags

| Build tag | Effect |
|-----------|--------|
| `ebpf_bindata` | `GetReader` serves assets from a generated Go blob (`bindata`). Used for official releases so no filesystem access is needed. |
| `!ebpf_bindata` (default) | `GetReader` opens files from the configured BPF directory on disk. Used during development and CI. |
| `linux_bpf` | Required for `runtime/` sub-package. Runtime compilation is only possible on Linux. |

### Linux-specific requirements

- Runtime compilation (`runtime/` sub-package) requires `clang` on `PATH` and kernel header packages for the running kernel.
- The `ProtectedFile` implementation uses `memfd_create` (Linux 3.17+) to keep source files in anonymous memory, preventing modification between integrity verification and compilation.
- `VerifyAssetPermissions` checks that eBPF object files are owned by `root:root` with at most `0o644` permissions.

## Usage

The overwhelming majority of eBPF probes in the agent load their object file like this:

```go
// Disk mode (development/CI):
bc, err := bytecode.GetReader(config.BPFDir, "network.o")
if err != nil { /* handle */ }
defer bc.Close()
collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
```

For runtime-compiled assets (e.g., network tracer with kernel-specific offsets), callers use the `runtime` sub-package:

```go
asset := runtime.NewAsset("tracer.c", expectedHash)
out, err := asset.Compile(ebpfConfig, []string{"-DSOME_FLAG"})
if err != nil { /* handle */ }
defer out.Close()
collectionSpec, err := ebpf.LoadCollectionSpecFromReader(out)
```

Example callers in the codebase: `pkg/network/ebpf/bpf_module.go`, `pkg/network/dns/ebpf.go`, `pkg/gpu/probe.go`, `pkg/collector/corechecks/ebpf/probe/*/probe.go`.

---

## Integration with the loading strategy fallback chain

`bytecode` is the **delivery layer** that sits below the three loading strategies described in
[pkg/ebpf](../ebpf.md):

| Strategy | How `bytecode` is involved |
|---|---|
| CO-RE (`LoadCOREAsset`) | `GetReader` opens the `.o` from `$BPF_DIR/co-re/` (or bindata). The `AssetReader` is passed directly to `ebpf-manager.InitWithOptions`. |
| Runtime compilation (`bytecode/runtime`) | `asset.Compile` produces an in-memory `CompiledOutput` that also satisfies `AssetReader`. |
| Prebuilt | `GetReader` opens the `.o` from `$BPF_DIR/` (or bindata). |

The `ebpf_bindata` build tag selects which implementation of `GetReader` is compiled in;
callers do not need to change when switching between bindata and disk mode.

### Security model

`VerifyAssetPermissions` enforces that every disk-mode `.o` file is owned by `root:root`
with permissions ≤ `0o644`. This prevents a non-root process from replacing an eBPF object
with a malicious one between the permission check and `InitWithOptions`. The `ProtectedFile`
in `runtime/` extends this model to source files by copying them into an anonymous memory
file (`memfd_create`) before any modification callback runs.

## Related packages

- [pkg/ebpf](../ebpf.md) — orchestrates all three loading strategies and calls `GetReader` / `asset.Compile`.
- [pkg/ebpf/maps](maps.md) — consumes `AssetReader` outputs once the collection spec is loaded.
- [pkg/network](../network/network.md) — largest consumer; see `pkg/network/ebpf/bpf_module.go` for how network tracer selects CO-RE vs runtime-compiled vs prebuilt objects.
