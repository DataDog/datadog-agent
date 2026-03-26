> **TL;DR:** `pkg/security/serializers` converts internal CWS event objects into the JSON payloads sent to the Datadog SIEM backend — it is the authoritative source of the public CWS event wire format and backend JSON schema.

# pkg/security/serializers

## Purpose

`pkg/security/serializers` turns internal CWS (Cloud Workload Security) event objects (`model.Event`, `events.CustomEvent`) into JSON payloads that are sent to the Datadog SIEM backend and displayed in the Security Signals UI. It is the authoritative source of the public JSON schema for CWS events.

Code generation (`go:generate`) produces the corresponding [backend schema documentation](../../../docs/cloud-workload-security/backend_linux.schema.json) and [easyjson](https://github.com/mailru/easyjson) marshal/unmarshal code for performance-critical hot paths.

## Platform support

| File | Platforms |
|------|-----------|
| `serializers_linux.go` | Linux (full event coverage) |
| `serializers_windows.go` | Windows (subset of event types) |
| `serializers_base.go` | Linux + Windows (shared types) |
| `serializers_others.go` | All other platforms (stub — returns empty serializers) |
| `helpers.go` | All platforms (shared helper generics) |

All types annotated `// easyjson:json` have machine-generated marshalers in the adjacent `*_easyjson.go` files.

## Key elements

### Key functions

#### Top-level entry points

| Function | Description |
|----------|-------------|
| `NewEventSerializer(event, rule, scrubber) *EventSerializer` | Builds the complete serializer tree for a `model.Event`. The `rule` parameter is optional; if supplied it populates `RuleContext` (expression + matching sub-expressions). `scrubber` is applied to string values before they are stored. |
| `MarshalEvent(event, rule, scrubber) ([]byte, error)` | Convenience wrapper: calls `NewEventSerializer` then `utils.MarshalEasyJSON`. |
| `MarshalCustomEvent(event) ([]byte, error)` | Serializes an `events.CustomEvent` (agent-generated events such as rate-limit notices, heartbeats, self-test results). Calls `CustomEvent.MarshalJSON` which defers to the `EventMarshaler` supplied at construction time. |
| `NewBaseEventSerializer(event, rule, scrubber) *BaseEventSerializer` | Builds the platform-independent portion of the serializer (event context, process tree, exit event). |
| `UnmarshalEvent(raw []byte) (*model.Event, error)` | (Linux) Reconstructs a `model.Event` from a serialized JSON payload. Currently supports exec events only; used by the ptracer and test tooling. |
| `DecodeEvent(file string) (*model.Event, error)` | (Linux) Reads a JSON file from disk and calls `UnmarshalEvent`. Useful for offline event replay and testing. |

### Key types

#### Serializer hierarchy (Linux)

```
EventSerializer
├── BaseEventSerializer
│   ├── EventContextSerializer      (evt)
│   │   └── []MatchedRuleSerializer
│   │   └── RuleContext (expression, matching subexpressions)
│   ├── ProcessContextSerializer    (process)
│   │   ├── ProcessSerializer       (current process)
│   │   └── []ProcessSerializer     (ancestors, up to 200 deep)
│   ├── FileEventSerializer         (file)
│   ├── ExitEventSerializer         (exit)
│   └── ContainerContextSerializer  (container)
├── NetworkContextSerializer        (network)
├── DDContextSerializer             (dd — APM span/trace IDs)
├── SecurityProfileContextSerializer (security_profile)
├── CGroupContextSerializer         (cgroup)
└── <event-type-specific serializer>
    e.g. DNSEventSerializer, BPFEventSerializer, SELinuxEventSerializer, …
```

#### Shared types (`serializers_base.go`, Linux + Windows)

| Type | JSON key | Description |
|------|----------|-------------|
| `BaseEventSerializer` | top-level | Common fields present in every event: `evt`, `date`, `file`, `exit`, `process`, `container`. |
| `EventContextSerializer` | `evt` | Event name, category, outcome, async flag, matched rules, rule context, source. |
| `MatchedRuleSerializer` | inside `evt.matched_rules` | Rule ID, version, tags, policy name/version. Used for anomaly-detection events. |
| `RuleContext` | `evt.rule_context` | Full SECL expression and `[]MatchingSubExpr` (offset, length, field, scrubbed value). |
| `ProcessContextSerializer` | `process` | Current process + parent + ancestors list; `truncated_ancestors` flag when the tree exceeds 200 entries. |
| `ContainerContextSerializer` | `container` | Container ID, creation time, rule variables. |
| `CGroupContextSerializer` | `cgroup` | CGroup ID, manager, rule variables. |
| `NetworkContextSerializer` | `network` | Device, L3/L4 protocol, source/destination IP:port, size, direction, type. |
| `DNSEventSerializer` | `dns` | DNS query/response: ID, question (class, type, name, size, count), response code. |
| `IMDSEventSerializer` | `imds` | IMDS (Instance Metadata Service) events; AWS sub-struct with IMDSv2 flag and scrubbed credentials. |
| `AWSSecurityCredentialsSerializer` | inside `imds.aws` | Code, type, `access_key_id`, expiration. |
| `ExitEventSerializer` | `exit` | Exit cause (`EXITED`, `SIGNALED`, `COREDUMPED`) and exit/signal code. |
| `RawPacketSerializer` | `packet` | Raw packet capture: network context, TLS version, dropped flag, decoded layers. |
| `NetworkFlowMonitorSerializer` | `network_flow_monitor` | Aggregated flow stats (ingress/egress bytes and packet counts) per device. |
| `SysCtlEventSerializer` | `sysctl` | sysctl action, parameter name, old/new value, truncation flags. |
| `Variables` | (various) | `map[string]interface{}` of rule-scoped variables, scrubbed before serialization. |
| `EventStringerWrapper` | — | `fmt.Stringer` wrapper that marshals a `model.Event` or `events.CustomEvent` via `MarshalEvent`/`MarshalCustomEvent`; useful for structured logging. |

#### Linux-specific types (selected)

| Type | JSON key | Description |
|------|----------|-------------|
| `FileSerializer` | `file` | Full file metadata: path, inode, mode, UID/GID, xattrs, flags, timestamps, package info, hashes, OverlayFS layer, mount info, ELF metadata. |
| `ProcessSerializer` | inside `process` | PID, TID, credentials (full UID/GID/EUID/EGID/FSUID/FSGID/AUID + cap sets), args/envs (truncated), executable and interpreter file info, cgroup, container, user session (k8s/SSH), APM tracer context. |
| `CredentialsSerializer` | inside `process.credentials` | Real, effective, filesystem UIDs/GIDs plus capability sets. |
| `UserSessionContextSerializer` | `process.user_session` | K8s exec session (username, UID, groups, extra) or SSH session (client IP, port, auth method, public key). |
| `DDContextSerializer` | `dd` | APM span ID and trace ID, propagated up the ancestor chain if not present on the event itself. |
| `SELinuxEventSerializer` | `selinux` | SELinux boolean/enforce changes. |
| `BPFEventSerializer` | `bpf` | BPF program and map metadata. |
| `SyscallContextSerializer` | `syscall` | Per-event syscall arguments (chmod, chown, open, exec, etc.). |
| `SetSockOptEventSerializer` | `setsockopt` | Socket type, level, option name, BPF filter instructions and hash. |

## Usage

### In the CWS probe

`pkg/security/probe/probe_ebpf.go` calls `serializers.MarshalEvent(event, rule, scrubber)` when a rule matches and the resulting JSON is forwarded to `pkg/security/module/server.go`, which sends it to the Datadog backend.

```go
data, err := serializers.MarshalEvent(event, matchedRule, probe.scrubber)
if err != nil {
    log.Errorf("failed to serialize event: %v", err)
    return
}
// data is sent to the security intake endpoint
```

### Scrubbing

The `scrubber` (`*utils.Scrubber`) is applied to every string field that could contain sensitive data (command-line arguments, environment variables, rule-matching values). The scrubber rules are configured via `system-probe.yaml`.

### Schema generation

The backend JSON schema is regenerated by running `go generate` inside the package:

```bash
go generate ./pkg/security/serializers/
```

This invokes `pkg/security/generators/backend_doc` and writes `docs/cloud-workload-security/backend_linux.schema.json`. The schema is consumed by the Datadog backend for validation and by documentation tooling.

### Adding a new event type

1. Define a new `*EventSerializer` struct annotated `// easyjson:json`.
2. Add a case to the `switch eventType` block in `NewEventSerializer`.
3. Embed the new serializer as a pointer field in `EventSerializer` with a `json:"..."` tag.
4. Run `go generate ./pkg/security/serializers/` to regenerate easyjson code and the backend schema.

### Key interfaces

#### `EventSerializerPatcher` interface (`patcher.go`)

```go
type EventSerializerPatcher interface {
    model.DelayabledEvent
    PatchEvent(*EventSerializer)
}
```

Events that implement `EventSerializerPatcher` can mutate the `EventSerializer` after it is built but before marshalling. This is used to attach delayed or asynchronously resolved data (e.g. hash results or remote-config enrichment) to the JSON payload.

## Related documentation

| Doc | Description |
|-----|-------------|
| [security.md](security.md) | Top-level CWS overview: how `MarshalEvent` is called by `APIServer` and the resulting JSON is forwarded to the Datadog SIEM backend. |
| [events.md](events.md) | `CustomEvent` and `AgentContext`/`BackendEvent` types that `MarshalCustomEvent` and `NewEventSerializer` consume. |
| [secl-model.md](secl-model.md) | `model.Event` struct that `NewEventSerializer` takes as input; `FieldHandlers` are invoked to lazily resolve fields before serialization. |
| [probe.md](probe.md) | `EBPFProbe` calls `MarshalEvent` when a rule fires; the scrubber passed in is configured via `system-probe.yaml`. |
| [../../pkg/util/scrubber.md](../../pkg/util/scrubber.md) | `*utils.Scrubber` passed to `NewEventSerializer`; applied to command-line args, env vars, and rule-matching values before they enter the JSON payload. |
