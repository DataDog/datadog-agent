# pkg/util/lsof

## Purpose

`pkg/util/lsof` lists the files and sockets open by a given process. Its primary
use-case is populating the agent **flare** with a snapshot of the agent's own
file-descriptor table and memory-mapped files, helping support engineers diagnose
fd leaks, unexpected open sockets, or missing shared libraries.

The implementation is platform-specific:

- **Linux** — reads `/proc/<pid>/fd`, `/proc/<pid>/maps`, and
  `/proc/<pid>/net/{tcp,tcp6,udp,udp6,unix}` via the `prometheus/procfs` library.
  Socket inodes are cross-referenced with the net files to produce human-readable
  `host:port->host:port` descriptions and TCP/UDP state strings.
- **Windows** — uses `EnumProcessModules` / `GetModuleFileNameEx` to list loaded
  DLLs. A separate JSON report (`ListLoadedModulesReportJSON`) enriches each entry
  with PE build timestamp, on-disk file timestamp, and Windows version-info strings
  (company name, product name, version, etc.).
- **macOS / other** — returns `ErrNotImplemented`.

The `HOST_PROC` environment variable can redirect the Linux implementation to an
alternative `/proc` mount (useful when the agent runs in a container and mounts
the host's procfs).

## Key elements

### Types

| Symbol | Description |
|--------|-------------|
| `File` struct | A single open file entry: `Fd` (descriptor label), `Type` (REG, DIR, SOCKET, PIPE, DEV, CHAR, LINK, tcp, tcp6, udp, udp6, unix, DLL, …), `OpenPerm` (permission bits of the fd/mapping), `FilePerm` (permission bits of the underlying file), `Size int64`, `Name` (path or socket address). |
| `Files` (`[]File`) | Slice of `File` with a `String()` method that formats the list as an aligned table for inclusion in a flare text file. |
| `LoadedModule` struct | Windows-only. Describes a single loaded DLL: path, build timestamp (PE COFF header), on-disk modification time, size, and Windows version-info strings. |
| `LoadedModulesReport` struct | Windows-only. Top-level JSON envelope (generated-at timestamp, process name/PID, slice of `LoadedModule`) written as `agent_loaded_modules.json` in the flare. |

### Errors

| Symbol | Description |
|--------|-------------|
| `ErrNotImplemented` | Returned by `openFiles` on platforms where listing is not supported (currently macOS and other non-Linux, non-Windows platforms). |

### Functions

| Symbol | Platform | Description |
|--------|----------|-------------|
| `ListOpenFiles(pid int) (Files, error)` | Linux, Windows | Lists open files/DLLs for the given PID. |
| `ListOpenFilesFromSelf() (Files, error)` | Linux, Windows | Convenience wrapper that calls `ListOpenFiles(os.Getpid())`. |
| `ListLoadedModulesReportJSON() ([]byte, error)` | Windows only | Builds and JSON-marshals a `LoadedModulesReport` for the current process. Returns `(nil, nil)` on non-Windows platforms. |

### Environment variable

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST_PROC` | `/proc` | Overrides the procfs root used by the Linux implementation. Set by the container agent to point at the host's procfs. |

## Usage

The package is consumed exclusively through the `comp/core/lsof` component
(`comp/core/lsof/impl/lsof.go`), which registers a **flare provider**. When a
flare is generated, the provider calls `ListOpenFilesFromSelf()` and writes the
result to `<flavor>_open_files.txt`. On Windows it additionally calls
`ListLoadedModulesReportJSON()` and writes `agent_loaded_modules.json`.

```go
import "github.com/DataDog/datadog-agent/pkg/util/lsof"

files, err := lsof.ListOpenFilesFromSelf()
if err != nil {
    if errors.Is(err, lsof.ErrNotImplemented) {
        // platform not supported, skip silently
    }
    return err
}
// files.String() produces a tab-formatted table
fmt.Println(files.String())
```

Direct use outside the flare component is uncommon. If you need to inspect
another process's open files (e.g. for diagnostic checks), call
`lsof.ListOpenFiles(pid)` directly.

## Related packages

| Package / component | Relationship |
|---|---|
| [`comp/core/flare`](../../comp/core/flare.md) | The `comp/core/lsof` component registers a **flare provider** (via `flaretypes.NewProvider`) that calls `ListOpenFilesFromSelf()` and writes `<flavor>_open_files.txt` into every flare archive. On Windows it also writes `agent_loaded_modules.json`. The flare component orchestrates collection but delegates the actual fd/DLL enumeration entirely to this package. |
| [`pkg/util/filesystem`](filesystem.md) | Complementary file-system utilities. While `lsof` reports what file descriptors a *process* has open, `pkg/util/filesystem` provides helpers for reading, copying, and securing files *on disk*. Some of the files enumerated by `lsof` (e.g., log paths, socket paths) are also managed by `filesystem` helpers in other parts of the agent. |
