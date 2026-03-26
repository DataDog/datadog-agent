> **TL;DR:** Provides cross-platform helpers for file I/O, permission management, disk-usage queries, and a concurrency-safe artifact generation pattern that prevents multiple processes from regenerating the same file simultaneously.

# pkg/util/filesystem

## Purpose

`pkg/util/filesystem` is a collection of cross-platform helpers for common file
system operations used throughout the agent: reading and copying files,
managing permissions, querying disk usage, and safely writing shared artifacts
under concurrency.

The package hides platform differences behind a single API. Permission
management, file rights checks, and disk queries all have Unix and Windows
implementations selected by build tags; callers never need to branch on
`runtime.GOOS`.

## Key Elements

### Key functions

#### Basic File Utilities (`file.go`, `common.go`)

| Function | Description |
|---|---|
| `FileExists(path string) bool` | Returns `true` if the path is stat-able. |
| `ReadLines(filename string) ([]string, error)` | Reads a file line-by-line into a string slice. |
| `CopyFile(src, dst string) error` | Atomically copies `src` to `dst` via a temp file + rename. Preserves source permissions. |
| `CopyFileAll(src, dst string) error` | Like `CopyFile` but creates missing parent directories first. |
| `CopyDir(src, dst string) error` | Recursively copies a directory tree. |
| `EnsureParentDirsExist(p string) error` | Creates all missing parent directories for a path. |
| `OpenFileForWriting(path string) (*os.File, *bufio.Writer, error)` | Opens (or creates) a file for writing with a buffered writer. |
| `GetFileSize(path string) (int64, error)` | Returns the file size in bytes. |
| `GetFileModTime(path string) (time.Time, error)` | Returns the file modification time. |
| `OpenShared(path string) (*os.File, error)` | On Windows, opens a file with `FILE_SHARE_DELETE` to allow rotation while the file is held open. On Unix, delegates to `os.Open`. |

### Key types

#### Permissions (`permission_nowindows.go` / `permission_windows.go`)

```go
type Permission struct{ /* platform-specific fields */ }

func NewPermission() (*Permission, error)
func (p *Permission) RestrictAccessToUser(path string) error
func (p *Permission) RemoveAccessToOtherUsers(path string) error
```

`Permission` is a cross-platform abstraction for restricting file access:

- **Unix**: `RestrictAccessToUser` chowns the file to the `dd-agent` user/group
  if that user exists; `RemoveAccessToOtherUsers` additionally strips all group
  and other mode bits (result: `0700` permissions for the owning user).
- **Windows**: `RestrictAccessToUser` replaces the file ACL so only the
  `dd-agent` user, `SYSTEM`, and `Administrators` retain `GENERIC_ALL` access.
  `RemoveAccessToOtherUsers` calls `RestrictAccessToUser` directly.

On Unix, if the `dd-agent` user does not exist or if `chown` is denied (e.g.
the process is not root), `RestrictAccessToUser` returns `nil` and does not
fail — the caller is expected to carry on.

#### File Rights Validation (`rights_nix.go` / `rights_windows.go`)

```go
// Unix only
func CheckRights(path string, allowGroupExec bool) error
```

Used by the secrets backend (`comp/core/secrets`) to validate that an
executable helper has safe permissions before running it. On Unix it asserts
that the file has no group-write or other-write bits set (and optionally no
group-read bits either), and that it is executable by the current process.

#### Disk Usage (`disk.go` / `disk_usage.go` / `disk_windows.go`)

```go
type DiskUsage struct {
    Total     uint64
    Available uint64
}

type Disk struct{}

func NewDisk() Disk
func (Disk) GetUsage(path string) (*DiskUsage, error)
```

A thin wrapper around `gopsutil/disk.Usage` (Unix) and the Windows API. Returns
total and available bytes for the filesystem hosting `path`.

### Key interfaces

#### Concurrent Artifact Management (`concurrent_write.go`)

This is the most sophisticated part of the package. It solves the problem of
multiple agent processes (or goroutines) trying to generate and cache the same
artifact at the same time.

```go
type ArtifactBuilder[T any] interface {
    Generate() (T, []byte, error)
    Deserialize([]byte) (T, error)
}

func TryFetchArtifact[T any](location string, factory ArtifactBuilder[T]) (T, error)
func FetchArtifact[T any](ctx context.Context, location string, factory ArtifactBuilder[T]) (T, error)
func FetchOrCreateArtifact[T any](ctx context.Context, location string, factory ArtifactBuilder[T]) (T, error)
```

`FetchOrCreateArtifact` is the main entry point. Its logic:

1. Try to deserialize the artifact from `location`.
2. If it does not exist, acquire a file lock on `location + ".lock"`.
3. After acquiring the lock, check again (another process may have created it
   meanwhile).
4. If still absent, call `factory.Generate()`, write the result to a temp file,
   `fsync`, set permissions to `dd-agent`, then atomically rename it into place.
5. The lock file is cleaned up on exit.

`FetchArtifact` is a blocking variant that retries every 500 ms until the
context is cancelled. `TryFetchArtifact` is a single non-blocking attempt.

The retry delay is `500ms` (`retryDelay` constant); the lock file suffix is
`".lock"` (`lockSuffix` constant).

## Platform Notes

| File | Constraint |
|---|---|
| `permission_nowindows.go` | `//go:build !windows` |
| `permission_windows.go` | `//go:build windows` |
| `rights_nix.go` | `//go:build !windows` |
| `rights_windows.go` | `//go:build windows` |
| `open_nix.go` | `//go:build !windows` |
| `open_windows.go` | `//go:build windows` |
| `disk.go` | `//go:build !windows` |
| `disk_windows.go` | `//go:build windows` |

## Usage

The package is used broadly across the agent for different concerns:

- **Secret resolution** (`comp/core/secrets/impl/fetch_secret.go`): calls
  `CheckRights` before executing a secret helper, and `Permission` to secure
  the secrets socket.
- **Certificate management** (`pkg/api/security/cert/cert_getter.go`): uses
  `FetchOrCreateArtifact` to generate and cache TLS certificates with safe
  multi-process semantics.
- **Forwarder** (`comp/forwarder/defaultforwarder`): uses `EnsureParentDirsExist`
  and disk helpers for retry queue storage.
- **Security agent / eBPF** (`pkg/security/ebpf/kernel`): uses `FileExists` and
  `ReadLines` for kernel header detection.
- **Config environment** (`pkg/config/env/environment.go`): uses `FileExists`
  for container runtime detection.
- **SNMP profiles** (`pkg/collector/corechecks/snmp`): uses `ReadLines` and
  file helpers when loading YAML profile files from disk.

### Typical artifact pattern

```go
type myCert struct{ /* ... */ }

type certBuilder struct{}
func (certBuilder) Generate() (myCert, []byte, error) { /* create key+cert, PEM-encode */ }
func (certBuilder) Deserialize(b []byte) (myCert, error) { /* parse PEM */ }

cert, err := filesystem.FetchOrCreateArtifact(ctx, "/opt/datadog-agent/run/auth.pem", certBuilder{})
```
