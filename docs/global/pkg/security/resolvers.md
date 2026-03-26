# pkg/security/resolvers

## Purpose

`pkg/security/resolvers` provides a collection of stateful caches that translate raw kernel identifiers (inode numbers, mount IDs, PIDs, cgroup IDs, network namespace IDs, …) into human-readable, structured data. Resolvers are populated at startup via a _snapshot_ that walks `/proc`, and are kept up-to-date in real time by consuming eBPF events.

The top-level aggregate types (`EBPFResolvers`, `EBPFLessResolvers`, `WindowsResolvers`) hold one instance of every resolver and are created once per probe. All security event field handlers call into these resolvers to fill the structured fields (e.g. `process.file.path`, `container.id`, `process.argv`) that SECL rules match against.

## Key Elements

### `EBPFResolvers` (`resolvers_ebpf.go`)

The main aggregate for the Linux eBPF probe. Build tags: `linux`.

```go
type EBPFResolvers struct {
    DentryResolver       *dentry.Resolver
    MountResolver        mount.ResolverInterface
    ProcessResolver      *process.EBPFResolver
    NamespaceResolver    *netns.Resolver
    CGroupResolver       *cgroup.Resolver
    HashResolver         *hash.Resolver
    TagsResolver         *tags.LinuxResolver
    PathResolver         path.ResolverInterface
    SBOMResolver         *sbom.Resolver
    UserGroupResolver    *usergroup.Resolver
    UserSessionsResolver *usersessions.Resolver
    FileMetadataResolver *file.Resolver
    SignatureResolver    *sign.Resolver
    TCResolver           *tc.Resolver
    DNSResolver          *dns.Resolver
    SyscallCtxResolver   *syscallctx.Resolver
    TimeResolver         *ktime.Resolver
}
```

Lifecycle: `NewEBPFResolvers(...)` → `Start(ctx)` → `Snapshot()` → used during event processing → `Close()`.

`Snapshot()` walks all running processes from `/proc`, sorts them by creation time, and calls `SyncCache` on the mount and process resolvers to pre-populate caches.

### `dentry/Resolver` — path resolution

Translates `(inode, mountID)` pairs into file path strings using a two-layer LRU cache backed by an eBPF map (`path_names`). When a path is not cached, the resolver falls back to a kernel-space request via eRPC (embedded RPC built on a shared memory segment). The resolver tracks hit/miss counters per resolution strategy (cache vs. eRPC vs. map lookup).

Key types:
- `PathEntry { Parent model.PathKey; Name string }` — one node in the path tree stored in the LRU.
- `ErrEntryNotFound` — returned when the inode is not in any cache.

The eRPC path is Linux-only and requires a mapped segment set up by the eBPF manager.

### `mount/Resolver` — mount-point tracking

Keeps an LRU (`mountsLimit = 100 000` entries) of `model.Mount` objects indexed by mount ID. Syncs from `/proc/self/mountinfo` on startup (or via the `listmount` syscall if `SnapshotUsingListMount` is enabled). Updated in real time when mount/umount events arrive from eBPF.

```go
type ResolverInterface interface {
    GetMountPath(mountID uint32, containerID string, pid uint32) (string, string, string, error)
    SyncCache() error
    ...
}
```

Also provides a `NoOpResolver` (used when path resolution is disabled) that satisfies the same interface.

### `process/EBPFResolver` — process cache

Maintains an in-memory tree of `model.ProcessCacheEntry` objects, one per live process, with parent–child relationships. Used to answer "what is the full ancestry of PID X?" at event time.

Sources of truth (in priority order):
1. eBPF `pid_cache` map — entries inserted by the kernel probe on `exec`/`fork`/`exit`.
2. `/proc` fallback — on cache miss, reads `/proc/<pid>/status`, `/proc/<pid>/cmdline`, etc. Rate-limited to avoid proc spam.

Constants: `procResolveMaxDepth = 16`, `procFallbackLimiterPeriod = 30s`.

Key helpers (`resolver_linux.go`):
- `IsKThread(ppid, pid)` — identifies kernel threads (PPID == 2 or PID == 2).
- `IsBusybox(pathname)` — detects busybox so symlink resolution is handled correctly.
- `GetProcessArgv(pr)` / `GetProcessArgv0(pr)` — extract argv from `ArgsEntry`.

### `cgroup/Resolver` — workload tracking

Maps PIDs to their container/cgroup context (`cgroupModel.CacheEntry`, which holds `ContainerID`, `WorkloadSelector`, and image tags). Fires `CGroupCreated` / `CGroupDeleted` events via a `utils.Notifier` so other resolvers (SBOM, user-group, tags) can react to container lifecycle.

```go
type ResolverInterface interface {
    AddPID(*model.ProcessCacheEntry)
    DelPID(uint32)
    GetWorkload(containerutils.ContainerID) (*cgroupModel.CacheEntry, bool)
    GetWorkloadByCGroupID(containerutils.CGroupID) (*cgroupModel.CacheEntry, bool)
    RegisterListener(Event, utils.Listener[*cgroupModel.CacheEntry]) error
}
```

LRU limits: 1024 host workload entries, 1024 container workload entries, 2048 cache entries.

### `hash/Resolver` — file hashing (Linux-only)

Computes cryptographic hashes of files on demand (triggered by the `hash` rule action or security profile activity dumps). Supports MD5, SHA-1, SHA-256, and ssdeep (fuzzy hash). Results are cached in an LRU keyed by `(path, cgroupID)`, with file metadata (inode, mtime, size) stored to detect staleness.

```go
type ResolverOpts struct {
    Enabled        bool
    MaxFileSize    int64
    HashAlgorithms []model.HashAlgorithm
    EventTypes     []model.EventType
}
```

Rate-limited via `golang.org/x/time/rate` (`HashResolverMaxHashRate`). A `SizeLimitedWriter` enforces `MaxFileSize` during the copy to the hash function.

The ssdeep cache is separate and keyed by `(inode, size, cheapHash)` to avoid recomputing expensive fuzzy hashes when cheaper hashes hit.

### `netns/Resolver` — network namespace tracking (Linux-only)

Stores open file handles for each network namespace ID so that TC (traffic control) classifiers can be attached/detached on container creation/deletion. Backed by an LRU of 1024 `NetworkNamespace` entries. Stale (lonely) namespaces are flushed after 30 seconds.

```go
func (nr *Resolver) SaveNetworkNamespaceHandle(nsID uint32, nsPath *utils.NSPath) (*NetworkNamespace, bool)
func (nr *Resolver) GetNetworkNamespace(nsID uint32) (*NetworkNamespace, bool)
```

Communicates with `tc/Resolver` to install BPF classifiers when a new namespace appears.

### `tags/Resolver` — container tag resolution

Translates `ContainerID` or `CGroupID` → Datadog tags (image name, image tag, kube labels, etc.) via the Datadog tagger. The `LinuxResolver` (Linux-specific) additionally maintains a workload selector cache and notifies listeners when a workload selector is fully resolved.

```go
type Resolver interface {
    Resolve(id containerutils.WorkloadID) []string
    ResolveWithErr(id containerutils.WorkloadID) ([]string, error)
    GetValue(id containerutils.WorkloadID, tag string) string
    Start(ctx context.Context) error
}
```

Events: `WorkloadSelectorResolved`, `WorkloadSelectorDeleted` — used by the SBOM resolver to trigger scans.

### `file/Resolver` — file metadata cache

Caches `model.FileMetadata` (architecture, ELF/PE/Mach-O file type, etc.) keyed by `(containerID, path, mtime)`. Enabled by `FileMetadataResolverEnabled`. Provides `ResolveFileMetadata(containerID, path, mtime)`. Cache size is 512 entries.

### Additional resolvers

| Sub-package | Purpose | Platform |
|-------------|---------|----------|
| `path/` | Combines dentry + mount resolvers to produce full path strings. `ResolverInterface` with `Resolve(mountID, pathKey, pid)`. | Linux |
| `envvars/` | Reads environment variables from `/proc/<pid>/environ` for a process. | Linux |
| `usergroup/` | Maps UID/GID to username/groupname, reads from container-specific `/etc/passwd` and `/etc/group`. | Linux / Windows |
| `usersessions/` | Resolves SSH user sessions from the kernel's SSH user-session eBPF map. | Linux |
| `sbom/` | Identifies the OS package that owns a given file path by cross-referencing SBOM data (DPKG, RPM, APK). | Linux |
| `selinux/` | Snapshots the SELinux enforce status into an eBPF map. | Linux |
| `sign/` | Validates macOS code signatures. | macOS/Unix |
| `tc/` | Manages TC (traffic-control) BPF classifiers per network namespace. | Linux |
| `dns/` | Caches DNS query→response pairs observed via eBPF. | Linux |
| `syscallctx/` | Maps syscall context IDs to event types for correct event labelling. | Linux |

## Usage

`EBPFResolvers` is created by `pkg/security/probe` at probe startup:

```go
resolvers, err := resolvers.NewEBPFResolvers(config, ebpfManager, statsd, scrubber, eRPC, opts)
resolvers.Start(ctx)
resolvers.Snapshot()
```

During event processing, `pkg/security/probe/field_handlers_ebpf.go` calls individual resolvers to lazily populate event fields:

```go
// Example: resolve file path on first access
func (fh *EBPFFieldHandlers) ResolveFilePath(ev *model.Event, f *model.FileEvent) string {
    path, err := fh.resolvers.PathResolver.Resolve(f.MountID, f.PathKey, ev.ProcessContext.Pid)
    ...
}
```

`pkg/security/security_profile` also holds a reference to `*EBPFResolvers` to call `HashResolver.ComputeHashes` when inserting process nodes into activity trees.

### Cross-package interactions

| This package interacts with | Via | Purpose |
|-----------------------------|-----|---------|
| `pkg/security/probe` | `probe_ebpf.go`, `field_handlers_ebpf.go` | Creates `EBPFResolvers`; field handlers call individual resolvers to lazily fill `model.Event` fields — see [probe.md](probe.md) |
| `pkg/security/secl/model` | `model.Event`, `model.ProcessCacheEntry`, `model.FileEvent`, … | All resolved data is written back into these SECL model structs, which are then evaluated by the rule engine — see [secl-model.md](secl-model.md) |
| `pkg/security/security_profile` | `Manager.resolvers` | The security profile manager holds a reference to `EBPFResolvers` for hash computation and activity-tree insertion — see [security-profile.md](security-profile.md) |
| `pkg/security/process_list` | `process_resolver.ProcessResolver` (Owner) | The process cache built by `process/EBPFResolver` is driven by the same `model.ProcessCacheEntry` graph abstracted by `pkg/security/process_list` — see [process-list.md](process-list.md) |

### Resolver data flow

```
kernel (eBPF ring-buffer)
  └─► probe.EBPFProbe.handleEvent
        └─► field_handlers_ebpf.EBPFFieldHandlers
              ├─► PathResolver.Resolve(mountID, pathKey, pid)
              │     ├─► dentry/Resolver  (inode→name LRU + eRPC fallback)
              │     └─► mount/Resolver   (mountID→mountpoint LRU)
              ├─► process/EBPFResolver.Resolve(pid)
              │     └─► model.ProcessCacheEntry (ancestry tree)
              ├─► cgroup/Resolver.GetWorkload(containerID)
              │     └─► cgroupModel.CacheEntry  (WorkloadSelector, tags)
              ├─► tags/LinuxResolver.Resolve(containerID)
              │     └─► Datadog tagger (image name, kube labels, …)
              ├─► hash/Resolver.ComputeHashes(path, …)
              │     └─► MD5 / SHA-1 / SHA-256 / ssdeep LRU cache
              └─► usergroup/Resolver  (UID/GID → name from container's /etc/passwd)
```

### Snapshot and real-time update model

The resolver layer has a two-phase update model:

1. **Snapshot phase** (`resolvers.Snapshot()`): called once after `Start`. Walks `/proc` to pre-populate `mount/Resolver` (from `/proc/self/mountinfo`) and `process/EBPFResolver` (all running PIDs). This ensures the cache is warm before any events are processed.
2. **Real-time phase**: eBPF events (fork, exec, exit, mount, umount, cgroup create/delete) are dispatched by the probe and update the relevant resolver caches in-place. The `cgroup/Resolver` fires `CGroupCreated` / `CGroupDeleted` notifications via a `utils.Notifier` that triggers downstream resolvers (SBOM, tags, user-group).

### Interaction with the SECL model

Every SECL field accessed in a rule expression maps to a `Resolve*` method in the `FieldHandlers` interface (generated in `pkg/security/secl/model/field_handlers_unix.go`). The `EBPFFieldHandlers` implementation in `pkg/security/probe` satisfies this interface by delegating to the resolver sub-packages described above. See [secl-model.md](secl-model.md) for the full field list and [secl.md](secl.md) for the evaluation engine.
