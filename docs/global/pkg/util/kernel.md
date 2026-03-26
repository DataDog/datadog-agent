# pkg/util/kernel

## Purpose

`pkg/util/kernel` provides Linux-specific kernel introspection utilities used throughout the Agent and system-probe. It covers:

- Detecting and parsing the running kernel version (encoded as `LINUX_VERSION_CODE`).
- Resolving the canonical paths to `/proc`, `/sys`, and `/boot`, accounting for container environments where the host filesystem is bind-mounted.
- Enumerating CPUs and processes.
- Detecting kernel lockdown mode (relevant for eBPF).
- Retrieving Linux distribution and architecture information.

Two sub-packages extend the root package:

- **`headers/`** — locating and downloading the kernel header files required for eBPF runtime compilation.
- **`netns/`** — enumerating and switching between Linux network namespaces.

All files in the root package are guarded by `//go:build linux`; `arch.go` is the only file without that constraint, since `Arch()` relies only on `runtime.GOARCH`.

---

## Key Elements

### Root package (`pkg/util/kernel`)

#### Types

| Type | Description |
|------|-------------|
| `Version` (`uint32`) | Compact kernel version number in `LINUX_VERSION_CODE` format: `(major<<16) + (minor<<8) + patch`. Supports `.Major()`, `.Minor()`, `.Patch()`, `.String()`. |
| `LockdownMode` (`string`) | Kernel security lockdown level; one of `None`, `Integrity`, `Confidentiality`, `Unknown`. |
| `UbuntuKernelVersion` | Struct for Ubuntu-specific version strings (`Major`, `Minor`, `Patch`, `Abi`, `Flavor`). |

#### Functions and variables

| Symbol | Description |
|--------|-------------|
| `HostVersion() (Version, error)` | Returns the running kernel version, detected from the vDSO ELF note (reliable across container environments). Result is memoized. |
| `MustHostVersion() Version` | Like `HostVersion()` but panics on error. |
| `ParseVersion(s string) Version` | Parses `"x.y.z"` into a `Version`. |
| `VersionCode(major, minor, patch byte) Version` | Constructs a `Version` from individual components. |
| `ParseReleaseString(s string) (Version, error)` | Parses a full `uname -r` release string (e.g. `"5.15.0-1-amd64"`). Includes workarounds for patch-level overflow in kernels 4.14 and 4.19. |
| `NewUbuntuKernelVersion(s string) (*UbuntuKernelVersion, error)` | Parses Ubuntu release strings such as `"5.15.0-105-generic"`. |
| `GetLockdownMode() LockdownMode` | Reads `/sys/kernel/security/lockdown`. |
| `ProcFSRoot` (memoized func) | Returns the procfs root (`/proc`, `/host/proc`, or `$HOST_PROC`). Container-aware. |
| `SysFSRoot` (memoized func) | Returns the sysfs root. Container-aware. |
| `BootRoot` (memoized func) | Returns the boot root. Container-aware. |
| `HostProc(...string) string` | Joins `ProcFSRoot()` with the provided path components. |
| `HostSys(...string) string` | Joins `SysFSRoot()` with the provided path components. |
| `HostBoot(...string) string` | Joins `BootRoot()` with the provided path components. |
| `RootNSPID` (memoized func) | Returns the current PID as seen from the root namespace (via `readlink /proc/self`). |
| `AllPidsProcs(procRoot string) ([]int, error)` | Lists all numeric entries in `procRoot` (all running PIDs). |
| `WithAllProcs(procRoot string, fn func(int) error) error` | Iterates over all PIDs and calls `fn` for each; stops on first error. |
| `GetProcessEnvVariable(pid int, procRoot, envVar string) (string, error)` | Reads a single environment variable from `/proc/<pid>/environ` without loading the entire environment. |
| `GetProcessMemFdFile(pid int, procRoot, name string, maxSize int) ([]byte, error)` | Reads a named `memfd` file from a process's open file descriptors. |
| `ProcessExists(pid int) bool` | Checks whether `/proc/<pid>` exists. |
| `PossibleCPUs` (memoized func) | Returns the maximum number of possible CPUs from `/sys/devices/system/cpu/possible`. |
| `OnlineCPUs` (memoized func) | Returns the slice of currently online CPU indices. |
| `Release` (memoized func) | Returns `uname -r`. |
| `Machine` (memoized func) | Returns `uname -m`. |
| `UnameVersion` (memoized func) | Returns `uname -v`. |
| `Platform`, `PlatformVersion`, `Family` (memoized funcs) | Linux distribution name, version, and family (via gopsutil). |
| `Arch() string` | Maps `runtime.GOARCH` to the kernel's architecture directory name (e.g. `"amd64"` → `"x86"`). |
| `ParseMountInfoFile(pid int32) ([]*mountinfo.Info, error)` | Parses `/proc/<pid>/mountinfo`. |
| `MountInfoPidPath(pid int32) string` | Returns the path to the mountinfo file for a PID. |

#### Version detection internals

The kernel version is read from the `LINUX_VERSION_CODE` field embedded in the **vDSO** ELF note section (via `/proc/self/mem`). This is more reliable than parsing `uname -r` because distros can modify the release string. The detection result is memoized with `sync.OnceValues`.

---

### `headers/` sub-package

**Build tag:** `linux && linux_bpf`

This package manages the lifecycle of kernel header files needed for eBPF **runtime compilation**. It is used by system-probe when CO-RE (Compile Once – Run Everywhere) is not available.

#### Key symbols

| Symbol | Description |
|--------|-------------|
| `HeaderOptions` | Configuration struct: `DownloadEnabled bool`, `Dirs []string` (custom header paths), `DownloadDir string`, `AptConfigDir`, `YumReposDir`, `ZypperReposDir`. |
| `GetKernelHeaders(opts HeaderOptions) []string` | Entry point. Returns validated header directory paths for the running kernel. The first call performs discovery (and possibly download); subsequent calls return the cached result. |
| `HeaderProvider` (`*headerProvider`) | Package-level singleton set by `GetKernelHeaders`. Exposes `GetResult()` for telemetry/diagnostics. |

**Header search order:**

1. Custom dirs (from `opts.Dirs`) validated against the running kernel version.
2. Default system locations: `/lib/modules/<release>/build`, `.../source`, `/usr/src/linux-headers-<release>`, `/usr/src/kernels/<release>`.
3. sysfs tarball at `/sys/kernel/kheaders.tar.xz` (requires `kheaders` kernel module / `CONFIG_KHEADERS`).
4. Previously downloaded headers in `DownloadDir`.
5. Download via the [nikos](https://github.com/DataDog/nikos) library (apt, yum/rpm, zypper, COS, WSL). Download is disabled unless `opts.DownloadEnabled` is true.

Validation checks that `include/linux/types.h` and `include/linux/kconfig.h` are present, and that the `LINUX_VERSION_CODE` in the headers matches the running kernel.

Telemetry counters (`ebpf__runtime_compilation__header_download`) are emitted on every attempt, tagged with platform, kernel version, architecture, and result.

---

### `netns/` sub-package

**Build tag:** `linux`

Utilities for working with Linux network namespaces using the `vishvananda/netns` library.

#### Key functions

| Function | Description |
|----------|-------------|
| `GetNetNamespaces(procRoot string) ([]netns.NsHandle, error)` | Returns deduplicated `NsHandle` values for every network namespace in use by running processes. Callers must `Close()` each handle. |
| `GetRootNetNamespace(procRoot string) (netns.NsHandle, error)` | Returns the network namespace of PID 1 (the host namespace). |
| `GetNetNamespaceFromPid(procRoot string, pid int) (netns.NsHandle, error)` | Opens the network namespace for a specific PID. |
| `GetNetNsInoFromPid(procRoot string, pid int) (uint32, error)` | Returns the inode number identifying the network namespace of a PID. |
| `GetCurrentIno() (uint32, error)` | Returns the inode of the current goroutine's network namespace. |
| `GetInoForNs(ns netns.NsHandle) (uint32, error)` | Returns the inode number for a given namespace handle. |
| `WithNS(ns netns.NsHandle, fn func() error) error` | Executes `fn` inside the specified namespace, then restores the previous namespace. Uses `runtime.LockOSThread()`. |
| `WithRootNS(procRoot string, fn func() error) error` | Convenience wrapper that enters the root network namespace before calling `fn`. |

**Important:** `WithNS` calls `runtime.LockOSThread()` because network namespace membership is per-thread in Linux. Do not use it from goroutines that must not be locked to an OS thread.

---

## Usage

### Checking kernel version for eBPF feature gating

```go
import "github.com/DataDog/datadog-agent/pkg/util/kernel"

kv, err := kernel.HostVersion()
if err != nil {
    return err
}
if kv < kernel.VersionCode(4, 14, 0) {
    // feature not available
}
```

### Container-aware procfs access

```go
path := kernel.HostProc(strconv.Itoa(pid), "net", "tcp")
// resolves to /host/proc/<pid>/net/tcp inside a containerized agent
```

### Fetching kernel headers for runtime compilation (system-probe)

```go
import "github.com/DataDog/datadog-agent/pkg/util/kernel/headers"

dirs := headers.GetKernelHeaders(headers.HeaderOptions{
    DownloadEnabled: cfg.EnableKernelHeaderDownload,
    DownloadDir:     cfg.KernelHeadersDownloadDir,
    AptConfigDir:    cfg.AptConfigDir,
    YumReposDir:     cfg.YumReposDir,
    ZypperReposDir:  cfg.ZypperReposDir,
})
// dirs is passed to the eBPF compiler as -I include paths
```

### Iterating network namespaces (network tracer)

```go
import "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"

nss, err := netns.GetNetNamespaces(kernel.ProcFSRoot())
for _, ns := range nss {
    defer ns.Close()
    _ = netns.WithNS(ns, func() error {
        // enumerate network connections inside this namespace
        return nil
    })
}
```

### Primary importers

The package is imported by ~80 packages, concentrated in:

- `pkg/network/` — connection tracking, DNS, HTTP monitoring.
- `pkg/security/` — eBPF-based security probes (CWS).
- `pkg/collector/corechecks/ebpf/` — eBPF core checks (OOM kill, TCP queue length, etc.).
- `cmd/system-probe/` — system-probe module entry points.
- `pkg/process/` — process monitoring.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/ebpf` | [pkg/ebpf.md](../ebpf.md) | The primary consumer of `pkg/util/kernel`. `ebpf.Config` reads `EnableKernelHeaderDownload` and delegates to `headers.GetKernelHeaders` for runtime compilation. `HostVersion()` is called by `LoadCOREAsset` and `Setup` to gate CO-RE and runtime-compilation paths. `ProcFSRoot()` / `SysFSRoot()` are used by the BTF loader, symbol table reader (`ksyms.go`), and lock-contention collector. |
| `pkg/ebpf/bytecode` | [pkg/ebpf/bytecode.md](../ebpf/bytecode.md) | The `runtime/` sub-package calls `headers.GetKernelHeaders` (via `pkg/util/kernel/headers`) to locate include paths before invoking `clang`. `ProtectedFile` uses `memfd_create` (Linux ≥ 3.17), which `HostVersion()` can be used to gate. |
| `pkg/network` | [pkg/network/network.md](../network/network.md) | The network tracer relies on `HostVersion()` to choose between kprobe, fentry, and CO-RE loading strategies. `ProcFSRoot()` and the `netns/` sub-package are used extensively by DNS monitoring, conntrack, and the per-namespace TCP/UDP socket enumeration path in `pkg/network/tracer/connection/`. |
| `pkg/security/probe` | [pkg/security/probe.md](../security/probe.md) | `EBPFProbe.Init()` calls `HostVersion()` to decide which kernel features are available (ring buffers, fentry, `bpf_probe_read_kernel`). The `constantfetch.BTFConstantFetcher` uses `SysFSRoot()` to find `/sys/kernel/btf/vmlinux`. The `resolvers/netns` sub-package calls `netns.GetNetNamespaces` and `netns.WithNS` to attach TC classifiers to new namespaces. |
