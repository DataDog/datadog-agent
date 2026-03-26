# pkg/security/agent

## Purpose

Implements the user-space security agent (`RuntimeSecurityAgent`): the process-side counterpart to the `system-probe` runtime security module. It connects to system-probe over gRPC, receives security events and activity dumps, forwards them to the Datadog backend via log reporters, and provides a command-line client interface for interacting with the module.

## Key elements

### Types

#### `RuntimeSecurityAgent` (`agent.go`)

Central struct. Owns:
- `statsdClient` — statsd metrics client
- `reporter` / `secInfoReporter` — `common.RawReporter` instances for CWS events and security-info events respectively (they target different log pipeline tracks; see [common.md](common.md))
- `eventClient *RuntimeSecurityEventClient` — gRPC streaming client for inbound events and activity dumps from system-probe
- `cmdClient *RuntimeSecurityCmdClient` — gRPC client for imperative commands (dump, reload policies, etc.)
- `storage ADStorage` — activity dump remote backend (Linux only; `nil` on Windows)
- `eventGPRCServer` — optional gRPC server when the agent owns the event socket (controlled by `runtime_security_config.event_grpc_server = "security-agent"`)
- `endpoints` / `secInfoEndpoints` — log intake endpoint configs, exposed to the `StatusProvider`
- Atomic counters: `running`, `connected`, `eventReceived`, `activityDumpReceived`

`RuntimeSecurityAgent` also implements the `SecurityAgentAPIServer` gRPC interface (`SendEvent`, `SendActivityDumpStream`) so it can act as the receiving server when `event_grpc_server` is set to `"security-agent"` (the agent listens for events rather than polling system-probe).

#### `ADStorage` interface

```go
type ADStorage interface {
    backend.ActivityDumpHandler
    SendTelemetry(_ statsd.ClientInterface)
}
```

Abstraction over the activity-dump remote backend. Implemented in `pkg/security/security_profile/storage/backend`.

#### `RSAOptions`

```go
type RSAOptions struct {
    LogProfiledWorkloads bool
}
```

Optional settings passed at construction time.

#### `RuntimeSecurityCmdClient` / `RuntimeSecurityEventClient` (`client.go`)

Two separate gRPC client structs:

- `RuntimeSecurityCmdClient` — wraps `api.SecurityModuleCmdClient`. Used for operator/diagnostic commands.
- `RuntimeSecurityEventClient` — wraps `api.SecurityModuleEventClient`. Provides streaming methods (`GetEventStream`, `GetActivityDumpStream`).

Both connect to the socket paths from `runtime_security_config.socket` / `runtime_security_config.cmd_socket`. The event client also supports vsock transport (used in VM/Fargate environments, configured via `vsock_addr`).

#### `SecurityModuleCmdClientWrapper` interface

Interface over `RuntimeSecurityCmdClient` exposing all diagnostic operations. Used for mocking in tests (`mocks/security_module_cmd_client_wrapper.go`).

Methods: `DumpDiscarders`, `DumpProcessCache`, `GenerateActivityDump`, `ListActivityDumps`, `StopActivityDump`, `GenerateEncoding`, `DumpNetworkNamespace`, `GetConfig`, `GetStatus`, `RunSelfTest`, `ReloadPolicies`, `GetRuleSetReport`, `GetLoadedPolicies`, `ListSecurityProfiles`, `SaveSecurityProfile`.

### Key functions

| Function | File | Description |
|----------|------|-------------|
| `NewRuntimeSecurityAgent(statsdClient, hostname)` | `agent_nix.go` / `agent_windows.go` | Platform-specific constructor. Linux version sets up the activity dump storage backend. Windows version skips telemetry and storage. Both call `setupGPRC()` to decide event transport mode. |
| `StartRuntimeSecurity(log, config, hostname, stopper, statsdClient, compression)` | `start.go` | Entry point called by the security-agent binary. Checks `runtime_security_config.enabled`; returns `nil` without error if disabled. Also returns `nil` when `runtime_security_config.direct_send_from_system_probe` is true (full system-probe mode). Creates reporters via `common.NewLogContextRuntime` / `common.NewLogContextSecInfo` and starts the agent. Build-gated to `linux \|\| windows`. |
| `(rsa) Start(reporter, endpoints, secInfoReporter, secInfoEndpoints)` | `agent.go` | Stores reporters, starts goroutines for event and activity-dump listeners (or starts the gRPC server if in server mode). Linux: also starts the activity dump storage telemetry loop (1-minute tick). |
| `(rsa) Stop()` | `agent.go` | Cancels context, sets `running=false`, closes clients, stops gRPC server, waits for goroutines. |
| `(rsa) DispatchEvent(evt)` | `agent.go` | Routes event to `secInfoReporter` (track `SecInfo`) or `reporter` (all others). The `Track` field on `SecurityEventMessage` is compared against `string(common.SecInfo)`. |
| `(rsa) DispatchActivityDump(msg)` | `agent.go` | Forwards an activity dump to the `ADStorage` backend via `storage.HandleActivityDump(image, tag, header, data)`. No-ops if `storage == nil` (Windows). |
| `(rsa) StatusProvider()` | `status_provider.go` | Returns a `status.Provider` that exposes connection state, event counters, environment info (kernel lockdown, ring buffer, fentry, constant fetchers), self-test results, and policy status in the `datadog-agent status` output. |

### Build flags

| File | Build constraint |
|------|-----------------|
| `start.go` | `linux \|\| windows` |
| `agent_nix.go` | `linux` (implicit via filename convention plus explicit `//go:build linux`) |
| `agent_windows.go` | `windows` (implicit) |
| `start_unsupported.go` | all other platforms — provides a stub `StartRuntimeSecurity` that always returns `nil, nil` |

## Usage

`StartRuntimeSecurity` is called from `cmd/security-agent/subcommands/start/command.go` (Linux/macOS) and `cmd/security-agent/main_windows.go` (Windows) as part of the security-agent startup sequence.

The typical lifecycle is:

1. `StartRuntimeSecurity` checks `runtime_security_config.enabled` (and `direct_send_from_system_probe`) and calls `NewRuntimeSecurityAgent`.
2. Two `CWSReporter` instances are created: one from `common.NewLogContextRuntime` (main CWS events) and one from `common.NewLogContextSecInfo` (remediation/secinfo events).
3. `agent.Start(reporter, endpoints, secInfoReporter, secInfoEndpoints)` launches the event-stream listener goroutines (or starts the gRPC server).
4. The event-stream listener reconnects automatically if system-probe is unavailable, using an exponential back-off (initial 2 s, max 60 s) for connection-error logging.
5. `stopper.Add(agent)` ensures `Stop()` is called during graceful shutdown.

### Connection modes

The `setupGPRC()` helper selects the transport at startup:

- **Client mode** (default): `RuntimeSecurityEventClient` polls system-probe's event socket (`runtime_security_config.socket`) via a gRPC streaming call. This is the normal deployment.
- **Server mode**: when `runtime_security_config.event_grpc_server = "security-agent"`, the agent starts its own gRPC server and system-probe pushes events to the agent. Used in environments where the agent cannot initiate the connection.

vsock transport is used when `vsock_addr` is configured (VM guest/host and Fargate scenarios).

### CLI usage

`RuntimeSecurityCmdClient` is also used directly by the `security-agent` CLI subcommands (e.g. `security-agent runtime policy reload`) via `NewRuntimeSecurityCmdClient()`. The cmd client derives the command socket path using `common.GetCmdSocketPath`.

## Related documentation

| Doc | Description |
|-----|-------------|
| [security.md](security.md) | Top-level CWS overview: `RuntimeSecurityAgent` is the user-space counterpart to `CWSConsumer`; the event flow from probe to backend is shown in full. |
| [probe.md](probe.md) | `CWSConsumer` (system-probe side) owns the `APIServer` gRPC implementation; the `SecurityModuleEvent` stream that `RuntimeSecurityEventClient.GetEventStream` consumes is defined there. |
| [common.md](common.md) | `RawReporter`, `NewLogContextRuntime`, `NewLogContextSecInfo`, `SecInfo` track constant, and `GetCmdSocketPath` — all used by this package. |
| [../../comp/core/ipc.md](../../comp/core/ipc.md) | The security-agent daemon registers the IPC component (`ModuleReadWrite`) in its fx graph; the CWS gRPC sockets are separate from the IPC HTTP transport, but the IPC component is used by other security-agent CLI commands. |
