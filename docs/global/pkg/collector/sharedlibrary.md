> **TL;DR:** An experimental, build-tag-gated mechanism for running agent checks implemented as native shared libraries (Rust/C-ABI `.so`/`.dll`), loaded at runtime via `dlopen` and wired into the standard check and aggregator pipeline at loader priority 40.

# pkg/collector/sharedlibrary

Import paths:
- `github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi`
- `github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/sharedlibraryimpl`

## Purpose

Provides an experimental mechanism for running Agent checks implemented as native shared libraries (`.so` on Linux, `.dll` on Windows, `.dylib` on macOS) without requiring Python or Go recompilation. The primary use case is Rust-based checks compiled to C-ABI shared libraries, though any language capable of exporting C-compatible `Run` and `Version` symbols works.

The feature is gated behind the `sharedlibrarycheck` build tag, so it compiles to no-ops in all standard agent builds. Rust helper code and a working example check live in `rustchecks/`.

## Key elements

### Configuration and build flags

### Build tag

All active code in both sub-packages is compiled only when the build tag `sharedlibrarycheck` is present. When the tag is absent, `sharedlibraryimpl` compiles to a package with a single no-op `InitSharedLibraryChecksLoader()` function.

### Configuration keys

| Key | Default | Description |
|---|---|---|
| `shared_library_check.enabled` | `false` | Whether shared library checks are activated |
| `shared_library_check.library_folder_path` | platform default additional-checks path | Directory searched for shared library files |

### Key types

### `ffi` sub-package — C bridge

The `ffi` package is a Cgo layer that wraps `dlopen`/`dlsym` (POSIX) or `LoadLibrary`/`GetProcAddress` (Windows) to load and call symbols from a shared library at runtime.

**Required library contract:** a shared library must export two C-ABI symbols:

| Symbol | Signature | Required |
|---|---|---|
| `Run` | `void Run(char *check_id, char *init_config, char *instance_config, const aggregator_t *agg, const char **error)` | Yes |
| `Version` | `const char *Version()` | No — falls back to `"unversioned"` |

The `aggregator_t` struct (defined in `ffi.h`) holds five callbacks corresponding to `SubmitMetric`, `SubmitServiceCheck`, `SubmitEvent`, `SubmitHistogramBucket`, and `SubmitEventPlatformEvent`. These are bound at startup to the Go aggregator functions in `pkg/collector/aggregator`.

**Key Go types:**

```go
// Library holds a loaded shared library and cached symbol pointers.
type Library struct { ... } // opaque; created by Open, consumed by Run/Version/Close

// LibraryLoader is the interface used by the check layer to operate on libraries.
type LibraryLoader interface {
    Open(name string) (*Library, error)
    Close(lib *Library) error
    Run(lib *Library, checkID string, initConfig string, instanceConfig string) error
    Version(lib *Library) (string, error)
    ComputeLibraryPath(name string) string
}

// SharedLibraryLoader is the production implementation of LibraryLoader.
func NewSharedLibraryLoader(folderPath string) *SharedLibraryLoader
```

`ComputeLibraryPath` builds a path of the form `<folderPath>/libdatadog-agent-<name>.<ext>`. The `libdatadog-agent-` prefix avoids collisions with system-level shared libraries.

Note: calling `dlopen` twice for the same path returns the same handle (POSIX semantics), so multiple check instances share the library's global state. This is a known limitation of the current design.

### `sharedlibraryimpl` sub-package — check and loader

This sub-package wires the FFI layer into the Agent's check framework.

```go
// CheckLoaderName is the registered name of this loader ("sharedlibrary").
const CheckLoaderName = "sharedlibrary"

// CheckLoader implements check.Loader; registered with the collector at priority 40.
type CheckLoader struct { ... }

// Check implements check.Check; one instance per loaded library+config pair.
type Check struct { ... }

// InitSharedLibraryChecksLoader registers the loader with the collector.
// Called from comp/collector/collector/collectorimpl/collector.go.
func InitSharedLibraryChecksLoader()
```

`CheckLoader.Load` computes the library path from the check name, calls `ffi.Open`, constructs a `Check`, and calls `Configure` to parse YAML instance/init configs. `Check.Run` delegates to `ffi.Run` then calls `sender.Commit()`. `Check.Cancel` calls `ffi.Close` and prevents further `Run` calls.

### `rustchecks/` — Rust SDK

A Cargo workspace under `pkg/collector/sharedlibrary/rustchecks/`:

- `core/` — Re-usable Rust crate wrapping the C callbacks; provides `AgentCheck` with typed methods (`gauge`, `service_check`, `event`, …).
- `checks/example/` — Template check: exports `Run` and `Version` symbols, delegates to a `check()` function that uses the `AgentCheck` API.

To add a new Rust check: copy `checks/example`, rename the crate, add it to `workspace.members`, and implement the `check()` function.

## Usage

### Enabling the feature

The loader is always registered when the agent binary is built with `-tags sharedlibrarycheck`. Without the tag, `InitSharedLibraryChecksLoader` is a no-op and no loader is registered.

```go
// comp/collector/collector/collectorimpl/collector.go
sharedlibrarycheck.InitSharedLibraryChecksLoader()
```

### Library naming and placement

The agent discovers libraries by name. For a check named `mycheck`, it looks for:

```
<library_folder_path>/libdatadog-agent-mycheck.so   # Linux
<library_folder_path>/libdatadog-agent-mycheck.dll  # Windows
<library_folder_path>/libdatadog-agent-mycheck.dylib # macOS
```

### Writing a Rust check

```rust
// In checks/mycheckname/src/check.rs
pub fn check(check: &AgentCheck) -> Result<()> {
    check.gauge("my.metric", 42.0, &vec![], "", false)?;
    Ok(())
}
```

Compile with:

```
cargo build --release --package mycheckname
```

Copy the resulting `libmycheckname.<ext>` to the configured `library_folder_path` and schedule the check via a standard `conf.d/mycheckname.d/conf.yaml`.

### Testing

E2E tests are in `test/new-e2e/tests/agent-runtimes/checks/shared-library/` and exercise the full pipeline from library loading through metric submission using the `example` Rust check.

Unit tests for the Go FFI layer are in `ffi/library_loader_test.go`; tests for the check and loader are in `sharedlibraryimpl/check_test.go` and `sharedlibraryimpl/loader_test.go`.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/collector/check`](check.md) | `sharedlibraryimpl.Check` implements the `check.Check` interface; `sharedlibraryimpl.CheckLoader` implements `check.Loader`. `check.ErrSkipCheckInstance` is returned by the loader when the library file is not found, allowing the scheduler to try the next loader in the chain. |
| [`pkg/collector/loaders`](loaders.md) | `InitSharedLibraryChecksLoader` registers the loader in the global catalog at priority 40 — lower priority than the Python loader (20) and Go core loader (30), so shared-library checks are only selected when no other loader claims the name. Unlike the other loaders, registration is triggered by an explicit call rather than an `init()` hook. |
| [`pkg/collector/aggregator`](aggregator-bridge.md) | The five `aggregator_t` callbacks in `ffi.h` (`SubmitMetric`, `SubmitServiceCheck`, `SubmitEvent`, `SubmitHistogramBucket`, `SubmitEventPlatformEvent`) are bound at startup to the same Go functions exported by the `pkg/collector/aggregator` Cgo bridge. Shared-library checks therefore submit metrics through the identical pathway as Python checks. |
