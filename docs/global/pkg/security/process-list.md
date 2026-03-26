# pkg/security/process_list — shared process graph for CWS

## Purpose

`pkg/security/process_list` provides a **generic, owner-driven process graph** that CWS uses as the common data structure behind both the runtime process resolver and activity dump/profile trees. Rather than hardcoding the matching and caching semantics, it delegates key decisions to an `Owner` interface, making the same graph reusable for workloads that need different notions of process identity (e.g., PID-based identity for the runtime resolver vs. pathname-based identity for activity dumps).

The package is **Linux-only** (`//go:build linux`).

### Sub-packages

| Sub-package | Purpose |
|-------------|---------|
| `process_list` (root) | Core graph types: `ProcessList`, `ProcessNode`, `ExecNode`, interfaces `Owner` and `ProcessNodeIface`. |
| `activity_tree/` | `ActivityTree` — an `Owner` implementation for activity dumps and security profiles. Uses pathname + exec-time as the exec cache key; matches execs by pathname (and optionally argv). |
| `process_resolver/` | `ProcessResolver` — an `Owner` implementation for the runtime process cache. Uses PID+NSID as the process key; root nodes are PID 1 only. |

## Key elements

### Interfaces

#### `Owner`

Implemented by both `ActivityTree` and `ProcessResolver`. Tells `ProcessList` how to:

| Method | Description |
|--------|-------------|
| `IsAValidRootNode(process)` | Whether a process without a known parent may become a root node. `ProcessResolver`: only PID 1. `ActivityTree`: always true (TODO: runc/containerID check). |
| `ExecMatches(e1, e2)` | Whether two `ExecNode`s represent the same executable invocation. `ProcessResolver`: same pathname. `ActivityTree`: same pathname, optionally same argv. |
| `ProcessMatches(p1, p2)` | Whether two `ProcessNode`s are the same process. `ProcessResolver`: same PID+NSID. `ActivityTree`: delegates to `ExecMatches` on current execs. |
| `GetExecCacheKey(process)` | Returns a cache key for the exec. Key type differs per owner. |
| `GetProcessCacheKey(process)` | Returns a cache key for the process. |
| `GetParentProcessCacheKey(event)` | Returns the cache key for the process's parent (used for tree insertion). |
| `SendStats(client)` | Sends owner-specific telemetry. |

#### `ProcessNodeIface`

Implemented by both `ProcessNode` and `ProcessList` (the list acts as the virtual parent of root nodes). Provides a uniform tree traversal API: `GetCurrentParent`, `GetPossibleParents`, `GetChildren`, `GetCurrentSiblings`, `AppendChild`, `UnlinkChild`.

### Core types

#### `ProcessList`

Thread-safe (`sync.Mutex`) graph/cache of processes scoped to a workload selector.

| Field | Description |
|-------|-------------|
| `selector` | `cgroupModel.WorkloadSelector` — `imageName/imageTag` for dumps, `imageName/*` for profiles, `*/*` for the process resolver. |
| `validEventTypes` | Only events of these types are accepted by `Insert`. |
| `owner` | The `Owner` that controls matching and key derivation. |
| `execCache` | `map[interface{}]*ExecNode` — fast lookup of execs by owner-defined key. |
| `processCache` | `map[interface{}]*ProcessNode` — fast lookup of processes by owner-defined key. |
| `rootNodes` | `[]*ProcessNode` — top-level entries (processes with no known parent in the graph). |
| `Stats` | `ProcessStats` — running totals and current counts of process and exec nodes. |

Key methods:

| Method | Description |
|--------|-------------|
| `Insert(event, insertMissingProcesses, imageTag)` | Routes the event: deletes the node on `ExitEventType`, finds or inserts an exec/process on other types, delegates non-lifecycle events to `ExecNode.Insert`. |
| `DeleteCachedProcess(key, imageTag)` | Removes a process and all its children (recursively), cleaning up image tag tracking. |
| `Walk(f)` | Depth-first traversal; stops early if `f` returns `true`. |
| `GetCacheExec(key)` / `GetCacheProcess(key)` | Direct cache lookups. |
| `Debug(w)` | Prints tree contents to an `io.Writer`. |

#### `ProcessNode`

Represents a single OS process (identified by PID lifetime). A process may have executed multiple binaries in succession (exec chain), each represented by an `ExecNode`.

| Field | Description |
|-------|-------------|
| `Key` | Owner-assigned cache key, or a random `uint64` if the owner returns nil. |
| `ImageTags` | Image versions this node belongs to (for multi-version dump/profile tracking). |
| `CurrentParent` / `PossibleParents` | The observed current parent and all historically seen parents (handles reparenting). |
| `CurrentExec` / `PossibleExecs` | The current and all historically seen execs for this PID. |
| `Children` | Child `ProcessNode`s (forked processes). |
| `UserData` | Extensible field for owner-specific data (e.g., ref-count, release callbacks). |

#### `ExecNode`

Represents a single binary execution within a process. Embeds `model.Process` (SECL process model).

| Field | Description |
|-------|-------------|
| `Key` | Owner-assigned exec cache key, or random `uint64`. |
| `ProcessLink` | Back-pointer to the owning `ProcessNode`. |
| `MatchedRules` | Rules that fired for this exec. |

### `activity_tree.ActivityTree` (Owner implementation)

Process key: `{pid, nsid}`. Exec key: `{pid, nsid, execTime, pathnameStr}` (uses `argv[0]` for busybox). Optionally differentiates args for exec matching. Carries `DNSNames` and `SyscallsMask` top-level summaries.

### `process_resolver.ProcessResolver` (Owner implementation)

Process key: `{pid, nsid}`. Exec key: `{pid, nsid, execTime, pathnameStr}`. Root: only PID 1. Process matching: same PID+NSID. Exec matching: same pathname. Includes the helper `IsBusybox(pathname) bool`.

## Usage

`ProcessList` is designed to be instantiated once per workload selector and driven by a stream of `model.Event` values:

```go
// Create a process resolver-backed list covering the whole system
resolver := processresolver.NewProcessResolver()
pl := processlist.NewProcessList(
    cgroupModel.WorkloadSelector{Image: "*", Tag: "*"},
    []model.EventType{model.ForkEventType, model.ExecEventType, model.ExitEventType},
    resolver,
    statsdClient,
    scrubber,
)

// Feed events
pl.Insert(event, true, "")

// Walk all processes
pl.Walk(func(node *processlist.ProcessNode) bool {
    fmt.Println(node.CurrentExec.FileEvent.PathnameStr)
    return false // continue
})
```

Currently `process_resolver` and `activity_tree` are the only `Owner` implementations. The package is in active development — several methods (`Contains`, `SaveToFile`, `ToJSON`, `ToDOT`) have stub implementations marked `// TODO`.

### Relationship to `resolvers/process/EBPFResolver`

The `process_resolver.ProcessResolver` (`Owner` implementation) and the `resolvers/process.EBPFResolver` serve complementary roles:

- `resolvers/process.EBPFResolver` — the **system-probe-side** process cache. It is populated from the eBPF `pid_cache` map and `/proc` fallback and is used by `field_handlers_ebpf.go` to resolve `process.*` SECL fields at event evaluation time. It stores `model.ProcessCacheEntry` nodes directly.
- `process_resolver.ProcessResolver` — the **process-list-side** `Owner` that controls how `ProcessList` groups `ProcessNode`/`ExecNode` objects. It delegates matching and key derivation to the same PID+NSID semantics as the eBPF resolver but operates on the higher-level `ProcessList` graph abstraction.

Both share the same `model.Process` type as the underlying data representation (see [secl-model.md](secl-model.md)).

### Relationship to `security_profile/activity_tree`

`activity_tree.ActivityTree` is an `Owner` implementation optimised for **activity dumps and security profiles** rather than the runtime process cache. Its key difference is exec-matching by pathname (optionally including argv), which allows it to collapse multiple short-lived processes running the same binary into a single `ExecNode`. This is the mechanism that keeps activity-dump trees compact across container restarts.

The `ActivityTree` is the tree inside `profile.Profile` (see [security-profile.md](security-profile.md)); `process_list.ProcessList` provides the generic graph engine underneath it.

### WorkloadSelector scoping

| Owner | `WorkloadSelector` | Meaning |
|---|---|---|
| `process_resolver.ProcessResolver` | `{Image: "*", Tag: "*"}` | Whole-system view (one list per host) |
| `activity_tree.ActivityTree` (dump) | `{Image: "my-app", Tag: "v1.2"}` | Exact image version |
| `activity_tree.ActivityTree` (profile) | `{Image: "my-app", Tag: "*"}` | Wildcard tag — matches all versions |

### ImageTags and multi-version tracking

`ProcessNode.ImageTags` records which image versions (tags) a process node has been observed in. This allows a single `ProcessList` instance (e.g., a profile) to track processes across multiple concurrent versions of a workload without duplicating the graph structure. `DeleteCachedProcess(key, imageTag)` removes only the association with a specific image tag, keeping the node alive if other versions still reference it.

## Related documentation

| Doc | Description |
|-----|-------------|
| [resolvers.md](resolvers.md) | `process/EBPFResolver` is the system-probe-side process cache; `process_resolver.ProcessResolver` in this package is its counterpart for the process-list graph abstraction. The cross-package interaction is documented in the resolvers Cross-package interactions table. |
| [security-profile.md](security-profile.md) | `activity_tree.ActivityTree` is the other `Owner` implementation; `ProcessList` is the graph engine powering the activity tree inside `profile.Profile`. |
| [probe.md](probe.md) | `Probe.DispatchEvent` feeds `model.Event` values that are ultimately `Insert`-ed into `ProcessList` instances via `Manager.ProcessEvent`. |
| [secl-model.md](secl-model.md) | `model.Process`, `model.ProcessCacheEntry`, and `model.EventType` are the core types stored and routed by `ProcessList`. |
