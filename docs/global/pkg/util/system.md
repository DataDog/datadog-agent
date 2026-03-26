> **TL;DR:** A broad collection of system-level helpers for querying CPU count, network routes and IPs, Linux namespace membership, file descriptors, inode numbers, dynamic library availability, and transport-agnostic IPC socket utilities.

# pkg/util/system

## Purpose

A collection of system-level helpers used across the agent to query hardware and OS properties: logical CPU count, network routes and IP addresses, Linux namespace membership, file descriptor counts, inode numbers, and dynamic library availability. The sub-package `socket/` adds transport-agnostic socket helpers.

## Key elements

### CPU (`cpu.go`, `cpu_unix.go`, `cpu_windows.go`, `cpu_mock.go`)

| Symbol | Description |
|---|---|
| `HostCPUCount() int` | Returns the number of logical CPUs on the **host** (not the container view). The result is cached atomically after the first successful call. On failure it falls back to `runtime.NumCPU()`, retrying up to `maxHostCPUFailedAttempts` (3) times before permanently using `runtime.NumCPU()`. |

**Platform behaviour:**
- Unix: delegates to `gopsutil/v4/cpu.CountsWithContext` (reads `/sys/devices/system/cpu`).
- Windows: uses `runtime.NumCPU()` (CPU affinity-aware).
- Test builds (`//go:build test`): pre-loads the cache with a fixed value of `3` so tests are deterministic.

### Network (`network.go`, `network_linux.go`, `network_windows.go`, `network_stub.go`)

| Symbol | Build | Description |
|---|---|---|
| `NetworkRoute` struct | all | Holds one routing table entry: `Interface`, `Subnet`, `Gateway`, `Mask` (all as `uint64`). |
| `IsLocalAddress(address string) (string, error)` | all | Returns the address unchanged if it resolves to a loopback (`127.x.x.x` / `::1`), error otherwise. Used to validate IPC endpoint configuration. |
| `GetProcessNetDevInode(procPath string, pid int) (uint64, error)` | Linux | Stat-based inode retrieval for `/proc/<pid>/net/dev`. |
| `IsProcessHostNetwork(procPath string, netDevInode uint64) *bool` | Linux | Compares a process's net/dev inode to PID 1's to detect host-network mode. Returns `nil` when the PID-1 inode is unavailable (e.g., degraded `/proc`). |
| `ParseProcessRoutes(procPath string, pid int) ([]NetworkRoute, error)` | Linux | Parses `/proc/<pid>/net/route` (or `/proc/net/route` when `pid == 0`). |
| `GetDefaultGateway(procPath string) (net.IP, error)` | Linux | Returns the first route with `Subnet == 0` from the routing table. |
| `ParseProcessIPs(procPath string, pid int, filterFunc func(string) bool) ([]string, error)` | Linux | Extracts `/32 host LOCAL` addresses from `/proc/<pid>/net/fib_trie`, returning deduplicated IP strings. |

### Namespaces (`namespace_linux.go`)

| Symbol | Build | Description |
|---|---|---|
| `GetProcessNamespaceInode(procPath, pid, namespace string) (uint64, error)` | Linux | Stat-based inode for `/proc/<pid>/ns/<namespace>`. Requires `CAP_SYS_PTRACE` for non-self PIDs. |
| `IsProcessHostUTSNamespace(_ string, namespaceID uint64) *bool` | Linux | Compares the inode to the hardcoded host UTS namespace inode (`0xEFFFFFFE`). |

### File descriptors and inodes (`file_linux.go`)

| Symbol | Description |
|---|---|
| `GetFileInode(path string) (uint64, error)` | Returns the inode number of any path via `syscall.Stat_t`. |
| `CountProcessFileDescriptors(procPath string, pid int) (int, error)` | Counts open FDs for one PID by reading `/proc/<pid>/fd`. |
| `CountProcessesFileDescriptors(procPath string, pids []int) (uint64, bool)` | Aggregates FD counts across multiple PIDs; the boolean indicates whether every PID lookup failed. |

### Dynamic library check (`dlopen_linux.go`, `dlopen_other.go`)

| Symbol | Build | Description |
|---|---|---|
| `CheckLibraryExists(libname string) error` | Linux + cgo + !static | Opens the named shared library with `dlopen(RTLD_LAZY)` and immediately closes it. Returns an error if the library is not found. Used to gate functionality that requires optional native dependencies. |

The `dlopen_other.go` stub is a no-op that always returns `nil` on non-Linux or static builds.

### socket/ sub-package

Provides transport-agnostic helpers for agent IPC sockets.

| Symbol | Build | Description |
|---|---|---|
| `IsAvailable(path string, timeout time.Duration) (bool, bool)` | Unix | Checks existence and reachability of a Unix domain socket. Returns `(exists, reachable)`. Permission-denied counts as reachable because the socket path may be reclaimed later. |
| `IsAvailable(path string, timeout time.Duration) (bool, bool)` | Windows | Same signature, backed by `go-winio` named pipe dial. |
| `GetFamilyAddress(path string) string` | all | Returns `"unix"` for absolute paths, `"tcp"` otherwise. |
| `GetSocketAddress(path string) (string, string)` | all | Returns `(family, address)`, recognising `"unix"`, `"tcp"`, and `"vsock:<addr>"` prefixes. |
| `ParseVSockAddress(addr string) (uint32, error)` | !clusterchecks | Parses symbolic vsock CIDs: `"host"`, `"hypervisor"`, `"local"`. |

## Relationship to other packages

| Package | Role |
|---|---|
| [`pkg/util/kernel`](kernel.md) | The canonical Linux `/proc` path resolver. `pkg/util/system` builds on top of `kernel.ProcFSRoot()` / `kernel.HostProc()` when constructing procfs paths such as `/proc/<pid>/net/route` or `/proc/<pid>/fd`. Code that needs kernel version detection or namespace enumeration should use `pkg/util/kernel` directly. |
| [`comp/core/ipc`](../../comp/core/ipc.md) | The IPC component manages the lifecycle and TLS configuration of the agent's inter-process HTTP sockets. `socket.IsAvailable` and `socket.GetSocketAddress` from `pkg/util/system/socket` are used throughout the agent to check reachability of these sockets before attempting connections. The IPC transport itself is backed by the same Unix domain socket paths exposed here. |
| [`pkg/util/net`](net.md) | Covers FQDN resolution and a lighter-weight UDS datagram availability check (`IsUDSAvailable`). `pkg/util/system/socket.IsAvailable` is a more general socket reachability helper (supports Unix, TCP, and named pipes) used for agent IPC; `pkg/util/net.IsUDSAvailable` is the narrower check used before DogStatsD UDS connections. |

## Usage

`pkg/util/system` is imported widely across the agent:

- **Configuration** (`pkg/config/setup`, `pkg/config/env`): `IsLocalAddress` validates IPC endpoint addresses (see `GetIPCAddress` in `pkg/config/setup`); `HostCPUCount` informs the default number of check workers (`DefaultNumWorkers`).
- **Container / workload metadata collectors** (`comp/core/workloadmeta`, `comp/core/tagger`): network namespace helpers detect host-network mode and extract container IPs. `IsProcessHostNetwork` is called when building the `HostNetwork` field on `workloadmeta.Container` entities.
- **Security agent** (`pkg/security/*`): gRPC client/server use `socket.GetSocketAddress` and `socket.IsAvailable` to locate the security module socket (`runtime_security_config.socket`).
- **System tray / GUI** (`comp/systray`): `socket.IsAvailable` drives the "agent running" indicator by probing the agent CMD socket.
- **Process checks** (`pkg/process/checks`): container-RT check uses `IsProcessHostNetwork` to identify containers sharing the host network stack.
- **IPC component** (`comp/core/ipc`): the HTTP transport layer uses the socket family helpers (`GetFamilyAddress`, `GetSocketAddress`) to configure Unix domain socket and vsock connections.

```go
import "github.com/DataDog/datadog-agent/pkg/util/system"

cpus := system.HostCPUCount()

routes, err := system.ParseProcessRoutes("/proc", 0)

import "github.com/DataDog/datadog-agent/pkg/util/system/socket"

exists, reachable := socket.IsAvailable("/var/run/datadog/agent.sock", time.Second)
family, addr := socket.GetSocketAddress("/var/run/datadog/agent.sock")
// family == "unix", addr == "/var/run/datadog/agent.sock"
```

### Checking library availability before using optional features

```go
import "github.com/DataDog/datadog-agent/pkg/util/system"

if err := system.CheckLibraryExists("libssl.so.3"); err != nil {
    // OpenSSL not found — skip TLS-inspection eBPF program
}
```
