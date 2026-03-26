# pkg/security/secl/model

## Purpose

`pkg/security/secl/model` defines the complete Cloud Workload Security (CWS) event data model. It is the concrete implementation of the abstract `eval.Model` / `eval.Event` interfaces from `pkg/security/secl/compiler/eval`, and it serves three distinct roles:

1. **Data container** — Go structs that mirror the kernel event layout, populated from eBPF ring-buffer messages or Windows ETW events.
2. **Rule engine bridge** — generated `GetEvaluator` / `GetFieldValue` / `SetFieldValue` methods that expose every field to the SECL evaluator by dotted path (e.g., `open.file.path`, `process.uid`).
3. **Serialisation source** — the same structs are marshalled to JSON for security signal payloads.

Most accessor and deep-copy code is **generated** — do not edit `accessors_unix.go`, `field_accessors_unix.go`, `event_deep_copy_unix.go`, or their Windows equivalents by hand. Regenerate with `go generate` from within the model package.

---

## Build-tag split

| File suffix / tag | Platform |
|---|---|
| `_unix.go` / `//go:build unix` | Linux and macOS |
| `_windows.go` / `//go:build windows` | Windows |
| `_linux.go` | Linux only |
| `_linux_amd64.go` / `_linux_arm64.go` | Architecture-specific constants |
| `_common.go` | Platform-independent |

The `//go:generate` directives at the top of `model_unix.go` and `model_windows.go` run the `accessors` and `event_deep_copy` code generators to produce the accessor and deep-copy files for each platform.

---

## Key types

### Event and base contexts

| Type | Description |
|---|---|
| `Event` | Top-level event struct (platform-specific: `model_unix.go` / `model_windows.go`). Embeds `BaseEvent` and carries one field per event category. |
| `BaseEvent` | Fields common to all events: `Type`, `Timestamp`, `ProcessContext`, `SecurityProfileContext`, `FieldHandlers`, `Rules` (matched rules), `ActionReports`. |
| `ProcessContext` | Process ancestry chain: current `Process`, `Parent *Process`, and iteration over ancestors. |
| `ProcessCacheEntry` | A refcounted node in the process cache tree (`Ancestor`, `Children`). Has `Exit(time)` and `HasValidLineage()`. |
| `ContainerContext` | Container ID, creation timestamp, and tags. |
| `NetworkContext` | L3/L4 protocol, source/destination `IPPortContext`, network device. |
| `CGroupContext` | cgroup ID and path, used for container/cgroup scoping. |
| `SecurityProfileContext` | Which security profile (if any) was matched for this event. |

### Event-type structs (Linux/Unix)

Each event category has a dedicated struct. The `field:"<name>" event:"<name>"` struct tags declare the SECL field prefix and event-type filter used by the generated accessors.

**File integrity monitoring (FIM):**
`OpenEvent`, `ChmodEvent`, `ChownEvent`, `MkdirEvent`, `RmdirEvent`, `RenameEvent`, `UnlinkEvent`, `UtimesEvent`, `LinkEvent`, `SetXAttrEvent`, `SpliceEvent`, `MountEvent`, `ChdirEvent`

**Process lifecycle:**
`ExecEvent`, `ExitEvent`, `ForkEvent`, `SetuidEvent`, `SetgidEvent`, `CapsetEvent`, `SignalEvent`, `SetrlimitEvent`, `PrCtlEvent`, `CapabilitiesEvent`

**Kernel / security:**
`BPFEvent`, `PTraceEvent`, `MMapEvent`, `MProtectEvent`, `LoadModuleEvent`, `UnloadModuleEvent`, `SELinuxEvent`, `SysCtlEvent`, `CgroupWriteEvent`

**Network:**
`BindEvent`, `ConnectEvent`, `AcceptEvent`, `DNSEvent`, `IMDSEvent`, `RawPacketEvent`, `NetworkFlowMonitorEvent`, `FailedDNSEvent`, `SetSockOptEvent`

**On-demand / custom:**
`OnDemandEvent`

**Windows-only:**
`CreateNewFileEvent`, `DeleteFileEvent`, `WriteFileEvent`, `CreateRegistryKeyEvent`, `OpenRegistryKeyEvent`, `SetRegistryKeyValueEvent`, `DeleteRegistryKeyEvent`, `ChangePermissionEvent`

### EventType constants

`EventType` is a `uint32` iota. Key sentinel values:

| Constant | Meaning |
|---|---|
| `FirstEventType` | = `FileOpenEventType` — first valid kernel event |
| `LastEventType` | = `SyscallsEventType` — last CWS-monitored event |
| `FirstDiscarderEventType` / `LastDiscarderEventType` | Range of events that accept kernel-level discarders |
| `LastApproverEventType` | Last event that accepts kernel-level approvers |
| `CustomEventType` | Synthetic event types for internal use |
| `FirstWindowsEventType` / `LastWindowsEventType` | Windows-only event range |

Use `EventType.String()` to convert to the string form used in SECL rules (e.g., `"open"`, `"exec"`, `"dns"`).

### Model

`Model` is the concrete implementation of `eval.Model`. It holds:
- Optional `ExtraValidateFieldFnc` / `ExtraValidateRule` hooks for platform-specific validation.
- A `legacyFields` map for backward-compatible field renames (populated via `SetLegacyFields`).

`Model.NewEvent()` returns a fresh `*Event`. Generated methods on `*Model`:
- `GetEvaluator(field, regID, offset)` — returns the typed evaluator closure for a given dotted field path.
- `GetFieldRestrictions(field)` — returns which event types a field is restricted to (e.g., `network.*` only on `dns`/`imds`/`packet`).
- `GetEventTypes()` — returns all supported event type strings.

### FieldHandlers interface

`FieldHandlers` (defined in the generated `field_handlers_unix.go` / `field_handlers_windows.go`) is an interface with one `Resolve*` method per lazy field. Each method is called on first access and caches the resolved value back into the event struct. The default (non-test) implementation lives in `pkg/security/probe`.

`FakeFieldHandlers` (in `model_unix.go`) provides a stub for unit tests — it accepts a `map[uint32]*ProcessCacheEntry` to seed the process cache.

Calling `ev.ResolveFields()` eagerly resolves all fields for the current event type. `ResolveFieldsForAD()` does the same but skips fields tagged `opts:skip_ad` (anomaly-detection).

### ProcessCacheEntry lifecycle

`ProcessCacheEntry` is a refcounted node in the global process tree:

- Ancestry: `Ancestor *ProcessCacheEntry`, `Children []*ProcessCacheEntry`
- `setAncestor(parent)` wires parent/child and propagates context (container, cgroup, SSH session, AUID).
- `Exit(time)` stamps the exit time.
- `HasValidLineage()` walks ancestors to verify the chain reaches PID 1 without gaps.
- `Releasable` embed provides `AppendReleaseCallback` / `CallReleaseCallback` for reference-counted pool return.

### Constants

Platform-independent constants are in `consts_common.go`:

| Constant | Value / Meaning |
|---|---|
| `MaxSegmentLength` | 255 — maximum path segment length |
| `MaxPathDepth` | 1189 — maximum eBPF dentry resolver depth |
| `MaxBpfObjName` | 16 — BPF object name max length |
| `ContainerIDLen` | 64 (SHA-256 hex) |
| `MaxSymlinks` | 2 — number of symlinks captured per event |
| `MaxTracedCgroupsCount` | 128 |

`EventFlags*` bit constants on `BaseEvent.Flags` track async execution, activity-dump sampling, and security-profile membership.

Linux-specific constant tables (open flags, capabilities, syscall numbers, BPF commands, etc.) live in `consts_linux.go` and architecture-specific files. They are exposed to the rule engine as named constant sets (e.g., `Constants:"Open flags"`) via the `eval` package.

### Variables

`SECLVariables` (in `variables.go`) is the pre-seeded map of built-in SECL variables available in rule expressions:

| Name | Type | Value |
|---|---|---|
| `${process.pid}` | `int` | PID of the process triggering the event |
| `${builtins.uuid4}` | `string` | Fresh random UUID4 |

### Iterator helpers

`Iterator[T]` is a generic interface (`Front`, `Next`, `At`, `Len`) used by generated code to walk repeated fields (e.g., process ancestors, DNS answers). `newIterator` / `newIteratorArray` are internal helpers that collect iterator results while maintaining a `IteratorCountCache` entry for repeated evaluations of the same rule.

---

## Generated files — do not edit manually

| File | Generator | Content |
|---|---|---|
| `accessors_unix.go` | `go generate` → `accessors` binary | `Model.GetEvaluator`, `GetFieldMetadata`, `GetFieldValue`, `SetFieldValue`, `GetEventTypes`, `GetFieldRestrictions` |
| `field_accessors_unix.go` | `accessors` binary | Per-field getter functions called from `GetEvaluator` closures |
| `field_handlers_unix.go` | `accessors` binary | `FieldHandlers` interface declaration |
| `event_deep_copy_unix.go` | `event_deep_copy` binary | `Event.DeepCopy()` |
| `accessors_windows.go` | same, Windows | Windows equivalents |
| `model_string.go` | `stringer` | `HashState.String()` |

The generator is driven by `//go:generate` lines in `model_unix.go` / `model_windows.go` and reads struct tags of the form `field:"<dotted.path>,handler:<HandlerMethod>,opts:<flags>,weight:<int>"`.

---

## Usage in the codebase

- **Rule engine** (`pkg/security/rules/`): instantiates `model.Model`, passes it to `rules.NewRuleSet`, and receives `*model.Event` pointers at every kernel event.
- **Probe** (`pkg/security/probe/`): fills `Event` structs from eBPF ring-buffer messages, implements `FieldHandlers` for lazy resolution, and manages the `ProcessCacheEntry` tree.
- **Serialisers** (`pkg/security/serializers/`): marshal `*model.Event` to JSON for signal payloads sent to the Datadog backend.
- **Activity trees** (`pkg/security/security_profile/activity_tree/`): store `ProcessCacheEntry` references and use model types for profile construction.
- **Tests** (`pkg/security/tests/`): use `model.NewFakeEvent()` / `FakeFieldHandlers` to construct synthetic events for unit and functional tests.

### Adding a new event type

1. Add a new `XxxEvent` struct to `model_unix.go` with `field:"<prefix>"` and `event:"<name>"` struct tags.
2. Add the new field to the top-level `Event` struct (e.g., `Xxx XxxEvent`).
3. Add the new `EventType` constant to the `EventType` iota and its string form to `String()`.
4. Run `go generate` inside the package to regenerate `accessors_unix.go`, `field_accessors_unix.go`, `field_handlers_unix.go`, and `event_deep_copy_unix.go`.
5. Implement any lazy-resolution methods in `pkg/security/probe/field_handlers_ebpf.go` (they are declared in the generated `field_handlers_unix.go` interface).
6. Add a corresponding serializer case in `pkg/security/serializers/serializers_linux.go` and run `go generate` there to regenerate the backend JSON schema.

### Lazy vs. eager resolution

Fields tagged with `handler:<MethodName>` are resolved lazily — the `FieldHandlers` method is called only when a SECL rule actually reads that field during evaluation. This keeps the hot path minimal: the probe fills only the fields that come directly from the eBPF ring-buffer message; everything else is computed on demand. Call `ev.ResolveFields()` (or `ev.ResolveFieldsForAD()` for activity dumps) to eagerly resolve all fields, e.g., before serializing an event.

---

## Related documentation

| Doc | Description |
|-----|-------------|
| [secl.md](secl.md) | SECL language overview: how `Model`/`Event` interfaces are used by the parser and evaluator; policy/rule management built on top of this model. |
| [secl-compiler.md](secl-compiler.md) | `eval.Model` / `eval.Event` interface definitions; `GetEvaluator`, `ValidateField`, and `NewEvent` contracts that `model.Model` must satisfy. |
| [resolvers.md](resolvers.md) | `EBPFResolvers` implements `FieldHandlers`; each `Resolve*` method in `field_handlers_ebpf.go` populates a field of the `model.Event` struct. |
| [security-profile.md](security-profile.md) | Activity trees store `ProcessCacheEntry` references from this package; `ProcessNode.Process` is a `model.Process` copy. |
| [probe.md](probe.md) | `EBPFProbe` fills `model.Event` structs from eBPF ring-buffer messages and calls `ResolveFields()` / `ResolveFieldsForAD()` before dispatching. |
| [security.md](security.md) | Top-level CWS overview showing how the model integrates with the rule engine, probe, serializers, and security profiles. |
