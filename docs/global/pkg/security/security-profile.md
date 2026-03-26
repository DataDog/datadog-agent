# pkg/security/security_profile

## Purpose

`pkg/security/security_profile` implements _behavioral security profiles_ for the Datadog Cloud Workload Security (CWS) product. A security profile is a model of what a containerized workload _normally_ does. During a learning phase the agent collects an _activity dump_ (a detailed record of process executions, file accesses, DNS queries, syscalls, network activity, etc.). Once stabilized, the profile is pushed to the kernel and used to detect deviations — _anomalies_ — in real time.

The package also provides helpers to export profiles as SECL rules or seccomp profiles (`rules.go`).

Build tag: almost all files are `//go:build linux`. The `profile_unsupported.go` stubs cover other platforms.

## Sub-packages

| Sub-package | Responsibility |
|-------------|----------------|
| `activity_tree/` | In-memory tree of process nodes and their observable actions |
| `dump/` | `ActivityDump`: a profile in its active event-collection phase |
| `profile/` | `Profile`: the stabilized in-memory security profile object |
| `storage/` | Persistence backends (local directory, remote forwarder to security-agent) |

## Key Elements

### `Manager` (`manager.go`)

The single entry point for security profiles and activity dumps. Created once per probe in `pkg/security/probe/probe_ebpf.go`.

```go
type Manager struct {
    // Activity dump fields
    activeDumps    []*dump.ActivityDump
    snapshotQueue  chan *dump.ActivityDump
    localStorage   *storage.Directory
    remoteStorage  *storage.ActivityDumpRemoteStorageForwarder

    // Security profile fields
    profiles       map[cgroupModel.WorkloadSelector]*profile.Profile
    pendingCache   *simplelru.LRU[cgroupModel.WorkloadSelector, *profile.Profile]

    // Shared
    resolvers      *resolvers.EBPFResolvers
    pathsReducer   *activity_tree.PathsReducer
    ...
}
```

Key methods:

| Method | Description |
|--------|-------------|
| `NewManager(cfg, statsd, ebpf, resolvers, ...)` | Constructs the manager, looks up all required eBPF maps (traced PIDs, cgroups, rate limiters, security profiles). |
| `ProcessEvent(*model.Event)` | Called for every event that is sampled for activity dumps. Inserts the event into all matching active dumps; pauses kernel collection if a dump exceeds `ActivityDumpMaxDumpSize`. |
| `LookupEventInProfiles(*model.Event)` | Called for every security event to check whether it matches (or deviates from) the workload's security profile. Sets `event.SecurityProfileContext`. |
| `HasActiveActivityDump(*model.Event)` | Returns true if the event belongs to a workload currently being dumped. Used by the rule engine to tag events for retention. |
| `Start(ctx)` / `Stop()` | Lifecycle management. |

The `Manager` receives workload lifecycle events via a single ordered channel (`workloadEvents`) to guarantee that `WorkloadSelectorResolved` and `WorkloadSelectorDeleted` events are processed in order.

### `activity_tree` sub-package

#### `ActivityTree` (`activity_tree.go`)

A process tree enriched with every observable action. No locks — callers hold the owning `Profile`'s mutex.

```go
type ActivityTree struct {
    ProcessNodes        []*ProcessNode
    CookieToProcessNode *simplelru.LRU[cookieSelector, *ProcessNode]
    DNSNames            *utils.StringKeys
    SyscallsMask        map[int]int
    Stats               *Stats
    pathsReducer        *PathsReducer
    ...
}
```

Insertion entry point: `Insert(entry, event, resolvers, generationType)` — finds or creates the relevant `ProcessNode`, then delegates to per-node insertion methods for each event type.

**`NodeGenerationType`** tracks how a node was added:

| Value | Meaning |
|-------|---------|
| `Runtime` | Added while the dump was running |
| `Snapshot` | Added during the initial `/proc` snapshot |
| `ProfileDrift` | Added because the event deviated from the profile |
| `WorkloadWarmup` | Added during the warm-up period of a new profile version |

**`Owner` interface** — must be implemented by the dump or profile that owns the tree:

```go
type Owner interface {
    MatchesSelector(entry *model.ProcessCacheEntry) bool
    IsEventTypeValid(evtType model.EventType) bool
    NewProcessNodeCallback(p *ProcessNode)
}
```

#### `ProcessNode` (`process_node.go`)

Holds a copy of `model.Process` and maps of activities observed for that process:

```go
type ProcessNode struct {
    Process        model.Process
    GenerationType NodeGenerationType
    Files          map[string]*FileNode
    DNSNames       map[string]*DNSNode
    IMDSEvents     map[model.IMDSEvent]*IMDSNode
    NetworkDevices map[model.NetworkDeviceContext]*NetworkDeviceNode
    Sockets        []*SocketNode
    Syscalls       []*SyscallNode
    Capabilities   []*CapabilityNode
    Children       []*ProcessNode
}
```

When a `ProcessNode` is created, `HashResolver.ComputeHashes` is called to attach file hashes to the binary.

#### `PathsReducer` (`paths_reducer.go`)

Applies heuristic regex patterns to collapse noisy/ephemeral path components (e.g. log rotation suffixes, temp file names) into canonical forms before inserting `FileNode` entries. This prevents the tree from exploding with thousands of near-identical paths.

```go
type PathsReducer struct {
    patterns []PatternReducer
}
```

### `dump/ActivityDump` (`dump/activity_dump.go`)

An `ActivityDump` wraps a `*profile.Profile` with lifecycle state for the collection phase.

```go
type ActivityDump struct {
    Cookie     uint64
    Profile    *profile.Profile
    LoadConfig *atomic.Pointer[model.ActivityDumpLoadConfig]
    state      ActivityDumpStatus // Stopped | Disabled | Paused | Running
    ...
}
```

States: `Stopped` → `Disabled` → `Running` → `Paused` → `Stopped`.

`Insert(event, resolvers)` delegates to `profile.ActivityTree` after validating the event type and container selector. Returns `(inserted bool, treeSize int64, err)`.

`OnNeedNewTracedPid` callback — invoked when the tree inserts a new process node; the `Manager` uses this to register the new PID in the kernel eBPF `traced_pids` map.

### `profile/Profile` (`profile/profile.go`)

The consolidated security profile, shared between the dump and the stable profile phases.

```go
type Profile struct {
    ActivityTree    *activity_tree.ActivityTree
    Header          ActivityDumpHeader
    Metadata        mtdt.Metadata
    versionContexts map[string]*VersionContext   // keyed by image tag
    Instances       []*tags.Workload
    LoadedInKernel  *atomic.Bool
    ...
}
```

**`VersionContext`** holds per-image-tag state:

```go
type VersionContext struct {
    FirstSeenNano  uint64
    LastSeenNano   uint64
    EventTypeState map[model.EventType]*EventTypeState
    Syscalls       []uint32
    Tags           []string
}
```

**`EventTypeState`** tracks the per-event-type anomaly detection state:

```go
type EventTypeState struct {
    LastAnomalyNano uint64
    State           model.EventFilteringProfileState
}
```

Serialization/deserialization: `profile/protobuf.go` uses the `adproto` protobuf schema. `profile/json.go` provides a JSON representation. `profile/graph.go` generates Graphviz DOT output for debugging.

### `storage` sub-package

**`ActivityDumpStorage` interface** (`storage/storage.go`):

```go
type ActivityDumpStorage interface {
    GetStorageType() config.StorageType
    Persist(request config.StorageRequest, p *profile.Profile, raw *bytes.Buffer) error
    SendTelemetry(sender statsd.ClientInterface)
}
```

Two implementations:

| Type | Description |
|------|-------------|
| `Directory` | Local filesystem storage. Maintains an LRU of up to `maxProfiles` profiles on disk. Supports gzip compression. |
| `ActivityDumpRemoteStorageForwarder` | Forwards serialized dumps to the security-agent gRPC handler (`backend.ActivityDumpHandler`). |

Storage format is determined by `config.StorageFormat` (protobuf, JSON). Multiple formats can be configured simultaneously.

### SECL/seccomp rule generation (`rules.go`)

`GenerateRules(ads []*profile.Profile, opts SECLRuleOpts) []*rules.RuleDefinition` — converts activity dump profiles into SECL rule definitions. Options:

```go
type SECLRuleOpts struct {
    EnableKill bool     // add kill action to generated rules
    AllowList  bool     // generate allowlist-style rules
    Lineage    bool     // include process lineage in expressions
    ImageName  string
    ImageTag   string
    FIM        bool     // include file integrity monitoring paths
}
```

`SeccompProfile` / `SyscallPolicy` — types for generating seccomp profiles from the syscall mask recorded in the activity tree.

### Anomaly detection (`secprofs.go`)

`LookupEventInProfiles(*model.Event)` looks up the profile for the event's workload (by image name + wildcard tag), then delegates to `profile.ActivityTree` to check if the event is in the profile. Result is written into `event.SecurityProfileContext.Status` (`NoProfile`, `InProfile`, `UnstableEventType`, `AnomalyDetectionEvent`, etc.).

The manager tracks per-event-type filtering decisions in `eventFiltering` counters, emitted as `datadog.security_agent.runtime.security_profile.event_filtering` metrics.

## Usage

The `Manager` is created in `pkg/security/probe/probe_ebpf.go`:

```go
spManager, err = securityprofile.NewManager(cfg, statsd, ebpfManager, resolvers, kernelVersion, newEvent, dumpHandler, hostname)
spManager.Start(ctx)
```

During event processing (called from the probe's event loop):

```go
// For activity dump collection:
spManager.ProcessEvent(event)

// For anomaly detection:
spManager.LookupEventInProfiles(event)

// To decide if an event should be saved (not discarded by kernel-space):
if spManager.HasActiveActivityDump(event) {
    event.SetSavedByActivityDumps()
}
```

Tests in `pkg/security/tests/security_profile_test.go` use the E2E framework to assert that anomaly events arrive in fakeintake after a behavioral deviation.

### Cross-package interactions

| This package interacts with | Via | Purpose |
|-----------------------------|-----|---------|
| `pkg/security/resolvers` | `Manager.resolvers *EBPFResolvers` | Calls `HashResolver.ComputeHashes` when inserting new process nodes into activity trees — see [resolvers.md](resolvers.md) |
| `pkg/security/secl/model` | `model.Event`, `model.ProcessCacheEntry`, `model.SecurityProfileContext` | All activity-tree insertions and anomaly lookups consume SECL model types; anomaly results are written into `event.SecurityProfileContext` — see [secl-model.md](secl-model.md) |
| `pkg/security/rules` | `rules.RuleDefinition` (via `GenerateRules`), `rules.RuleEngine.RuleMatch` | Profile-derived SECL rules are injected into the rule engine via `bundled.PolicyProvider.SetSBOMPolicyDef`; `RuleMatch` tags events for activity dump retention — see [rules.md](rules.md) |
| `pkg/security/events` | `events.AnomalyDetectionRuleID`, `events.NewCustomEvent` | Anomaly detection events are shipped as `CustomEvent` values on the `anomaly_detection` pseudo-rule — see [events.md](events.md) |
| `pkg/security/process_list` | `activity_tree.ActivityTree` (Owner implementation) | The activity tree is one of two `Owner` implementations of the `ProcessList` abstraction — see [process-list.md](process-list.md) |
| `pkg/security/probe` | `probe_ebpf.go` creates `Manager`; probe calls `ProcessEvent` / `LookupEventInProfiles` | The probe drives the manager's event loop — see [probe.md](probe.md) |

### Activity dump lifecycle

```
cgroup/Resolver fires CGroupCreated
  └─► Manager.workloadEvents channel
        └─► Manager.handleWorkloadSelectorResolved
              └─► ActivityDump created (state: Running)
                    └─► probe registers dump in eBPF traced_pids map

probe event loop
  └─► Manager.ProcessEvent(event)
        └─► ActivityDump.Insert(event, resolvers)
              └─► ActivityTree.Insert(entry, event, resolvers, generationType)
                    └─► ProcessNode created
                          └─► HashResolver.ComputeHashes (resolvers)

ActivityDump reaches max size or timeout
  └─► state: Paused → Stopped
        └─► storage.Persist (local directory or remote forwarder)
              └─► profile/Profile serialised as protobuf or JSON
```

### Security profile and anomaly detection lifecycle

```
ActivityDump completed → Profile stabilised
  └─► Manager.profiles map (WorkloadSelector → *profile.Profile)
        └─► profile.LoadedInKernel = true
              └─► eBPF security_profiles map populated

probe event loop
  └─► Manager.LookupEventInProfiles(event)
        └─► profile.ActivityTree membership check
              └─► event.SecurityProfileContext.Status set:
                    NoProfile | InProfile | UnstableEventType | AnomalyDetectionEvent

AnomalyDetectionEvent status
  └─► rules.RuleEngine.RuleMatch tags event for anomaly reporting
        └─► eventSender.SendEvent on anomaly_detection pseudo-rule
              └─► rate-limited by AnomalyDetectionLimiter (events package)
```

### SECL rule generation from profiles

`GenerateRules(ads, opts)` converts stabilised `*profile.Profile` objects into `[]*rules.RuleDefinition`. The generated definitions are then injected into the `bundled.PolicyProvider` via `SetSBOMPolicyDef`, causing a silent rule reload (no `ruleset_loaded` heartbeat). Options control:

- **`AllowList`**: generate deny rules (`exec.file.path not in [...]`) vs. allow rules (`exec.file.path in [...]`).
- **`Lineage`**: include process ancestry constraints in expressions.
- **`FIM`**: include observed file-access paths in integrity monitoring rules.
- **`EnableKill`**: attach a `kill` action to matched rules.

The resulting `RuleDefinition` objects use the same SECL syntax as user-authored rules and are evaluated by the same `pkg/security/secl/rules` engine. See [rules.md](rules.md) and [secl-rules.md](secl-rules.md).
