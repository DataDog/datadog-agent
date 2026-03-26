> **TL;DR:** Provides cross-platform ZIP creation/extraction and `.tar.xz` walking/extraction with path-traversal protection, used for flare packaging and eBPF/BTF asset unpacking.

# pkg/util/archive

## Purpose

`pkg/util/archive` provides functions to create and extract ZIP and `.tar.xz` archives. It is a platform-independent utility package with no build tag restrictions.

The ZIP implementation is adapted from the `mholt/archiver` library (v3.5.1) but strips out unneeded functionality and adds explicit security mitigations. The `.tar.xz` support was added later for extracting compressed kernel header archives and similar assets.

## Key elements

### Key functions

#### ZIP — creation

```go
func Zip(sources []string, destination string) error
```

Creates a `.zip` archive at `destination` from one or more source paths. Each source can be a file or a directory (walked recursively). Symlinks are silently skipped to prevent escaping the archive root. The destination file must not already exist and must have a `.zip` suffix. Parent directories are created automatically.

Compression: Deflate at default level for files; Store (no compression) for directory entries.

#### ZIP — extraction

```go
func Unzip(source, destination string) error
```

Extracts a `.zip` file to `destination`. For each entry:
- Symlinks are skipped (security measure against symlink attacks).
- Paths are sanitized using `github.com/cyphar/filepath-securejoin` (`SecureJoin`) to prevent path traversal (zip-slip).
- Directories are created with mode `0755`; files are written with mode `0755`.

#### `.tar.xz` — walking

```go
var ErrStopWalk = errors.New("stop walk")

func WalkTarXZArchive(tarxzArchive string, walkFunc func(*tar.Reader, *tar.Header) error) error
func WalkTarXZArchiveReader(rdr io.Reader, walkFunc func(*tar.Reader, *tar.Header) error) error
```

Iterates every entry in a `.tar.xz` archive, calling `walkFunc` for each. Returning `ErrStopWalk` from the callback stops iteration cleanly (not an error). The `Reader` variant accepts any `io.Reader`, useful when the archive is already open or comes from a network stream.

#### `.tar.xz` — extraction

```go
func TarXZExtractFile(tarxzArchive, path, destinationDir string) error
func TarXZExtractAll(tarxzArchive, destinationDir string) error
func TarXZExtractAllReader(rdr io.Reader, destinationDir string) error
```

- `TarXZExtractFile` — extracts a single named file (by archive path) and stops iteration early via `ErrStopWalk`. Returns an error if the path is not found.
- `TarXZExtractAll` / `TarXZExtractAllReader` — extract all regular files from the archive.

All extraction functions sanitize destination paths with `SecureJoin` to prevent path traversal.

### Configuration and build flags

#### Security considerations

- Symlinks are skipped in both `Zip` (during creation) and `Unzip` (during extraction).
- All extraction functions use `SecureJoin` to confine output files within the destination directory, guarding against zip-slip attacks.
- `Zip` guards against infinite loops when the destination file falls inside a source directory.

#### Dependencies

- Standard library `archive/zip`, `archive/tar`, `compress/flate`
- `github.com/cyphar/filepath-securejoin` — safe path joining
- `github.com/xi2/xz` — XZ decompression (`.tar.xz` only)

## Usage

The package is used in several parts of the agent:

**Flare builder** (`comp/core/flare/helpers/builder.go`): calls `Zip` to bundle the collected diagnostic files into a single `.zip` archive that is uploaded to Datadog support.

**eBPF BTF handling** (`pkg/ebpf/btf.go`, `pkg/ebpf/rc_btf.go`): calls `TarXZExtractFile` / `TarXZExtractAll` to unpack BTF (BPF Type Format) files from `.tar.xz` archives shipped with the agent.

**Kernel header discovery** (`pkg/util/kernel/headers/find_headers.go`): uses the `.tar.xz` API to locate and extract kernel headers for eBPF probe compilation.

**Security cgroup tests** (`pkg/security/utils/cgroup_test.go`): uses ZIP for test fixture creation.

Typical flare usage:

```go
err := archive.Zip([]string{"/tmp/flare-dir"}, "/tmp/flare-output.zip")
```

Typical BTF extraction:

```go
err := archive.TarXZExtractFile("btf.tar.xz", "vmlinux", "/tmp/btf/")
```
