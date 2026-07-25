# Inter-process communication

-----

The Agent's processes form a hub-and-spoke topology: the core `agent` owns the authentication artifacts and serves an HTTPS REST + gRPC API on `localhost:5001`, and every satellite process (trace-agent, process-agent, security-agent, system-probe, otel-agent, ...) connects back to it for tags, workload metadata, remote config, hostname, and configuration sync. This page covers the auth token and IPC certificate lifecycle, the command-port mux, the `AgentSecure` gRPC service catalog, config propagation, and the definitive port/socket/named-pipe map for every process. Which binaries exist at all is covered in [Binaries and flavors](binaries.md).

## Key packages and files

| Path | Purpose |
|---|---|
| [`comp/core/ipc/def/component.go`](<<<SRC>>>/comp/core/ipc/def/component.go) | IPC component interface: auth token, TLS configs, HTTP client and middleware |
| [`comp/core/ipc/impl/ipc.go`](<<<SRC>>>/comp/core/ipc/impl/ipc.go) | Loads or creates the auth artifacts; logs the auth fingerprint |
| [`comp/core/ipc/httphelpers/middleware.go`](<<<SRC>>>/comp/core/ipc/httphelpers/middleware.go) | Bearer-token HTTP middleware enforced on every REST endpoint |
| [`pkg/api/security/security.go`](<<<SRC>>>/pkg/api/security/security.go) | `auth_token` file management; DCA token handling |
| [`pkg/api/security/cert/cert_getter.go`](<<<SRC>>>/pkg/api/security/cert/cert_getter.go) | `ipc_cert.pem` creation and loading |
| [`comp/api/api/apiimpl/server_cmd.go`](<<<SRC>>>/comp/api/api/apiimpl/server_cmd.go) | The CMD server: REST `/agent/*` + gRPC muxed on `cmd_port` |
| [`comp/api/api/apiimpl/server_ipc.go`](<<<SRC>>>/comp/api/api/apiimpl/server_ipc.go) | The optional mTLS-only config server (`agent_ipc.*`) |
| [`comp/api/grpcserver/impl-agent/grpc.go`](<<<SRC>>>/comp/api/grpcserver/impl-agent/grpc.go) | Builds the core agent gRPC server; requires client certificates |
| [`comp/api/grpcserver/helpers/grpc.go`](<<<SRC>>>/comp/api/grpcserver/helpers/grpc.go) | `NewMuxedGRPCServer`: routes gRPC vs REST on one TLS port |
| [`comp/api/api/def/component.go`](<<<SRC>>>/comp/api/api/def/component.go) | `agent_endpoint` provider group; `AuthorizedConfigPathsCore` allowlist |
| [`comp/core/configsync/impl/sync.go`](<<<SRC>>>/comp/core/configsync/impl/sync.go) | Satellite-side poller of the core agent's `/config/v1` |
| [`comp/core/configstream`](<<<SRC>>>/comp/core/configstream) / [`configstreamconsumer`](<<<SRC>>>/comp/core/configstreamconsumer) | gRPC push of config snapshots/updates to remote agents |
| [`comp/core/remoteagentregistry`](<<<SRC>>>/comp/core/remoteagentregistry) | Registry where co-agents (ADP, DDOT) register for status/flare aggregation |
| [`comp/core/tagger/impl-remote/remote.go`](<<<SRC>>>/comp/core/tagger/impl-remote/remote.go) | Remote tagger client used by satellite processes |
| [`pkg/system-probe/api/server/listener_unix.go`](<<<SRC>>>/pkg/system-probe/api/server/listener_unix.go) / [`listener_windows.go`](<<<SRC>>>/pkg/system-probe/api/server/listener_windows.go) | system-probe UDS / named-pipe listener and its permission model |
| [`pkg/fleet/daemon/local_api_unix.go`](<<<SRC>>>/pkg/fleet/daemon/local_api_unix.go) / [`local_api_windows.go`](<<<SRC>>>/pkg/fleet/daemon/local_api_windows.go) | Installer daemon local socket API |
| [`cmd/cluster-agent/api/server.go`](<<<SRC>>>/cmd/cluster-agent/api/server.go) | DCA API on 5005 with dual-token validation |
| [`pkg/config/setup/ipc_address.go`](<<<SRC>>>/pkg/config/setup/ipc_address.go) | `GetIPCAddress`/`GetIPCPort` (`cmd_host`, deprecated `ipc_address`) |
| [`pkg/proto/pbgo/core`](<<<SRC>>>/pkg/proto/pbgo/core) | Generated protobuf/gRPC definitions for `Agent`, `AgentSecure`, `RemoteAgent` |

## Auth artifacts

Two files, stored next to `datadog.yaml` (`/etc/datadog-agent/` on Linux), are the trust root for all agent-to-agent communication:

1. **`auth_token`** (override path: `auth_token_file_path`): 32 random bytes stored as a hex string. Every REST request between processes carries it as `Authorization: Bearer <token>`, enforced by `ipc.HTTPMiddleware` ([`middleware.go`](<<<SRC>>>/comp/core/ipc/httphelpers/middleware.go)).
1. **`ipc_cert.pem`** (override: `ipc_cert_file_path`): a self-signed certificate plus key, used for **both** server TLS and client mTLS. The core agent's gRPC server requires client certificates (`grpcutil.RequireClientCert` in [`grpc.go`](<<<SRC>>>/comp/api/grpcserver/impl-agent/grpc.go)), and the dedicated config server sets `tls.RequireAndVerifyClientCert` ([`server_ipc.go`](<<<SRC>>>/comp/api/api/apiimpl/server_ipc.go)).

Creation semantics live in [`comp/core/ipc/impl/ipc.go`](<<<SRC>>>/comp/core/ipc/impl/ipc.go). The component has three constructors:

1. `NewComponent` (used by the core agent) atomically creates the files if missing, retrying up to `auth_init_timeout` (default 30 s).
1. `NewReadOnlyComponent` (used by satellites) only loads the files, so satellites wait for the core agent to have created them.
1. `NewInsecureComponent` (used by `flare`/`diagnose`) always succeeds even with missing artifacts, at the cost of possibly failing later at the IPC handshake.

At startup, every process logs `successfully loaded the IPC auth primitives (fingerprint: ...)` — a SHA-256 over token plus certificates (`printAuthSignature`). Comparing this fingerprint across processes is the fastest way to diagnose "401 from my own agent" problems.

/// warning
The artifacts are created by the first process that is *allowed* to create them. If a CLI command runs as root before the agent service ever started, `auth_token` and `ipc_cert.pem` can end up root-owned, and the `dd-agent`-owned service then fails to read them. Mismatched fingerprints across processes are the symptom.
///

## The command port: one TLS listener, two protocols

The core agent's CMD server binds `cmd_host:cmd_port` (default `localhost:5001`) with the IPC TLS config. A single port serves both protocols: `helpers.NewMuxedGRPCServer` ([`helpers/grpc.go`](<<<SRC>>>/comp/api/grpcserver/helpers/grpc.go)) routes requests with content-type `application/grpc` to the gRPC server and everything else to the REST mux ([`server_cmd.go`](<<<SRC>>>/comp/api/api/apiimpl/server_cmd.go)). The listener can alternatively be a vsock address (`vsock_addr`, for VM/enclave setups) or a Unix socket (any `cmd_host` containing `/`).

**REST side**: all routes live under `/agent/` and are contributed by components through the `agent_endpoint` fx value group ([`comp/api/api/def/component.go`](<<<SRC>>>/comp/api/api/def/component.go)) — status, flare, config, secrets refresh, workload-list, tagger-list, stream-logs, metadata, dogstatsd stats, and so on. This is what the `agent status`, `agent flare`, and other CLI subcommands call (see [Status, health, and telemetry](../operations/introspection.md) and [Flare](../operations/flare.md)).

**gRPC side** ([`pkg/proto/pbgo/core`](<<<SRC>>>/pkg/proto/pbgo/core)): three services, all TLS, with per-RPC bearer-token auth on the secure ones and client-certificate verification on the connection:

| Service | Auth | RPCs (abridged) |
|---|---|---|
| `Agent` | none beyond TLS | `GetHostname` |
| `AgentSecure` | mTLS + bearer token | `TaggerStreamEntities`, `TaggerFetchEntity`, `TaggerGenerateContainerIDFromOriginInfo`, `WorkloadmetaStreamEntities`, `AutodiscoveryStreamConfig`, `ClientGetConfigs`/`GetConfigState` (+ HA/MRF variants), `DogstatsdCaptureTrigger`/`DogstatsdSetTaggerState`, `StreamConfigEvents`/`CreateConfigSubscription`, `WorkloadFilterEvaluate`, `GetHostTags`, `ReportHealthIssue`/`ResolveHealthIssue` |
| `RemoteAgent` | mTLS + bearer token | `RegisterRemoteAgent`, refresh and event reporting for co-agents |

The `AgentSecure` streams are the transport for the [remote tagger](../containers/tagger.md), remote [workloadmeta](../containers/workloadmeta.md), [autodiscovery](../checks/autodiscovery.md) config streaming to the cluster-check runners, [remote configuration](../configuration/remote-config.md) (`ClientGetConfigs`, also proxied by the trace-agent for tracer libraries), DogStatsD capture/replay, and the config stream. The `RemoteAgent` service is the inverse direction: on-host co-agents such as ADP (the Agent Data Plane) and [DDOT](../otel/ddot.md) register themselves so the core agent can aggregate their status and flares ([`comp/core/remoteagentregistry`](<<<SRC>>>/comp/core/remoteagentregistry), gated by `remote_agent.registry.enabled`).

gRPC message sizes are capped by `agent_ipc.grpc_max_message_size` (default 128 MiB — configstream snapshots are large), with a soft warning threshold at `agent_ipc.grpc_warning_message_size`.

## Config synchronization between processes

Satellite processes must see the same *resolved* configuration as the core agent — including values rewritten at runtime by [secrets resolution](../configuration/secrets.md), delegated auth, or API-key rotation. Three mechanisms exist; the full precedence story is in [the configuration system](../configuration/overview.md):

1. **configsync (pull)** — the core agent optionally runs a second, dedicated HTTPS server, the "IPC server" ([`server_ipc.go`](<<<SRC>>>/comp/api/api/apiimpl/server_ipc.go)), on `agent_ipc.host:agent_ipc.port` (default port `0` = disabled) or on a Unix socket (`agent_ipc.use_socket`, `agent_ipc.socket_path`). It is mTLS-only (client cert required) and serves exactly one thing: `/config/v1/*`, restricted to the allowlist `AuthorizedConfigPathsCore` ([`endpoint.go`](<<<SRC>>>/comp/api/api/apiimpl/internal/config/endpoint.go)) — `api_key`, `app_key`, `site`, `dd_url`, `additional_endpoints`, proxy settings, and other values the core agent may rewrite. Satellites run the `configsync` component ([`sync.go`](<<<SRC>>>/comp/core/configsync/impl/sync.go)), polling every `agent_ipc.config_refresh_interval` seconds (0 = disabled). The Kubernetes Helm chart enables this between the pod's containers; host installs leave it off by default.
1. **configstream (push)** — the `AgentSecure.StreamConfigEvents` RPC on the command port streams a full snapshot followed by incremental updates, keyed by the config's sequence ID and preserving each setting's original source. Consumers opt in via `remote_agent.configstream.consumer.enabled` and must hold a remote-agent-registry session.
1. **fetcher (diagnostics pull)** — [`pkg/config/fetcher/from_processes.go`](<<<SRC>>>/pkg/config/fetcher/from_processes.go) lets CLI commands (`agent flare`, `agent config`) pull the full scrubbed runtime config *from* each satellite via that process's own settings API (ports in the table below), using the shared HTTP client in [`pkg/config/settings/http/client.go`](<<<SRC>>>/pkg/config/settings/http/client.go).

/// note
There are two different "IPC servers" in the core agent and the names collide constantly: the CMD API on `cmd_port` (5001, bearer token + mTLS gRPC) and the mTLS-only config server on `agent_ipc.port` (off by default). The deprecated `ipc_address` setting is a third thing entirely — an alias of `cmd_host` ([`ipc_address.go`](<<<SRC>>>/pkg/config/setup/ipc_address.go)).
///

## Satellite-specific channels

**system-probe** serves plain HTTP — no TLS, no bearer token — over `system_probe_config.sysprobe_socket`: a UDS at `/opt/datadog-agent/run/sysprobe.sock` on Linux/macOS (mode 0720, owner-restricted so the `dd-agent` group can connect — [`listener_unix.go`](<<<SRC>>>/pkg/system-probe/api/server/listener_unix.go)) or the named pipe `\\.\pipe\dd_system_probe` on Windows (DACL limited to admins/SYSTEM plus the agent user — [`listener_windows.go`](<<<SRC>>>/pkg/system-probe/api/server/listener_windows.go)). Access control is purely filesystem/DACL permissions: anything that can open the socket can query all modules. Clients (core agent checks, process-agent, security-agent) use [`pkg/system-probe/api/client`](<<<SRC>>>/pkg/system-probe/api/client). One endpoint is the exception and does use the IPC bearer-token middleware: `POST /agent-restart` ([`cmd/system-probe/api/server.go`](<<<SRC>>>/cmd/system-probe/api/server.go)). The CWS event monitor additionally exposes its own gRPC API on `runtime_security_config.socket` (UDS on Linux, `localhost:3335` TCP on Windows), consumed by the security-agent — see [Workload Protection](../ebpf/cws.md).

**installer daemon** exposes a plain-HTTP local API on `/opt/datadog-packages/run/installer.sock` (Linux) or `\\.\pipe\DD_INSTALLER` (Windows) for the installer CLI and debugging ([`local_api_unix.go`](<<<SRC>>>/pkg/fleet/daemon/local_api_unix.go)) — see [Fleet automation](../deployment/fleet.md).

**Cluster Agent** (DCA) runs its own muxed HTTPS+gRPC listener on `cluster_agent.cmd_port` (5005, bound on all interfaces) with **two token domains** ([`server.go`](<<<SRC>>>/cmd/cluster-agent/api/server.go)): intra-pod endpoints accept the pod-local IPC `auth_token`, while the "external" paths used by node agents (cluster-level tagger streams, cluster-check dispatch) require the shared **DCA token** (`cluster_agent.auth_token`, a ≥32-character secret configured on both sides). External paths reject the local token. Node agents reach it through the `datadog-cluster-agent` Kubernetes Service. Details in [Cluster Agent](../containers/cluster-agent.md).

**Remote tagger / workloadmeta plumbing**: the default remote tagger target is the core agent's `cmd_port` ([`params.go`](<<<SRC>>>/comp/core/tagger/def/params.go)); process-agent, security-agent, and system-probe require it, while trace-agent and otel-agent use the optional variant ([`fx-optional-remote`](<<<SRC>>>/comp/core/tagger/fx-optional-remote/fx.go)) that degrades to a noop when no core agent is reachable. Remote workloadmeta collectors ([`collectors/internal/remote`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote)) stream over the same channel. Node agents consume the DCA's cluster-level tagger with the same protocol on 5005.

## The definitive port map

All listeners bind localhost unless noted. Every port is configurable; the table shows defaults.

| Port | Process | Setting | What |
|---|---|---|---|
| 5000 | core agent | `expvar_port` | expvar/pprof, optional Prometheus telemetry |
| 5000 | cluster-agent | `metrics_port` | DCA telemetry |
| 5000 | dogstatsd (standalone) | `dogstatsd_stats_port` | stats |
| 5001 | core agent | `cmd_port` | REST + gRPC command API (auth) |
| 5002 | core agent (Windows/macOS) | `GUI_port` | Web GUI (disabled, `-1`, on Linux) |
| 5003 | trace-agent | `otlp_config.traces.internal_port` | internal OTLP handoff from the core agent's embedded collector |
| 5005 | cluster-agent | `cluster_agent.cmd_port` | DCA REST + gRPC (0.0.0.0; node agents) |
| 5005 | CLC runner | `clc_runner_port` | DCA-to-runner stats (DCA dials out) |
| 5010 | security-agent | `security_agent.cmd_port` | runtime settings / status |
| 5011 | security-agent | `security_agent.expvar_port` | expvar |
| 5012 | trace-agent | `apm_config.debug.port` | expvar/pprof/config |
| 5555 | core agent (k8s) | `health_port` (default 0) | liveness/readiness probes |
| 6062 | process-agent | `process_config.expvar_port` | expvar |
| 6162 | process-agent | `process_config.cmd_port` | settings / status |
| 6262 | process-agent | `process_config.language_detection.grpc_port` | process-entity gRPC stream (used on Windows) |
| 7777 | otel-agent | `ddflareextension` ([`factory.go`](<<<SRC>>>/comp/otelcol/ddflareextension/impl/factory.go)) | config/flare introspection queried by the core agent |
| 7778 | host-profiler | `hostprofiler.hpflare.port` | flare |
| 8000 | cluster-agent | `admission_controller.port` | mutating webhooks (0.0.0.0) |
| 8125/udp | agent / dogstatsd / ADP | `dogstatsd_port` | StatsD ingest (`dogstatsd_non_local_traffic` for 0.0.0.0) |
| 8126/tcp | trace-agent | `apm_config.receiver_port` | APM / OTLP proxy / EVP ingest (`apm_config.non_local_traffic`) |
| 8443 | cluster-agent | `external_metrics_provider.port` | Kubernetes external metrics (0.0.0.0) |
| 9162 | core agent | `network_devices.snmp_traps.port` | SNMP traps ([NDM](../pipelines/ndm.md)) |
| 3335 | system-probe (Windows) | `runtime_security_config.socket` | CWS gRPC |
| 4317/4318 | otel-agent or core OTLP | collector config / `otlp_config.receiver` | OTLP gRPC/HTTP ingest |

### Sockets and named pipes

| Path (Linux / Windows) | Owner process | Protocol | Clients |
|---|---|---|---|
| `/opt/datadog-agent/run/sysprobe.sock` / `\\.\pipe\dd_system_probe` | system-probe | HTTP REST (no token) | core agent checks, process-agent, security-agent |
| `/opt/datadog-agent/run/runtime-security.sock` / `localhost:3335` | system-probe (CWS) | gRPC | security-agent |
| `/var/run/datadog/dsd.socket` | agent / dogstatsd / ADP | DogStatsD datagrams | apps, tracers, JMXFetch |
| `/var/run/datadog/apm.socket` / `apm_config.windows_pipe_name` | trace-agent (or trace-loader) | HTTP | tracers |
| `/opt/datadog-packages/run/installer.sock` / `\\.\pipe\DD_INSTALLER` | installer daemon | HTTP | installer CLI |
| `${run_path}/agent_ipc.socket` (opt-in) | core agent | HTTPS mTLS | configsync clients |
| procmgr runtime socket / named pipe | dd-procmgrd | gRPC | [`pkg/procmgr/coat`](<<<SRC>>>/pkg/procmgr/coat) clients (agent status) |

## Configuration

The settings governing IPC (all declared in [`pkg/config/setup`](<<<SRC>>>/pkg/config/setup)):

| Setting | Default | Meaning |
|---|---|---|
| `cmd_host` / `cmd_port` | `localhost` / `5001` | Command API bind address; must resolve to a local address (`GetIPCAddress` enforces this) |
| `ipc_address` | — | Deprecated alias of `cmd_host` |
| `auth_token_file_path` / `ipc_cert_file_path` | next to `datadog.yaml` | Auth artifact locations |
| `auth_init_timeout` | 30 s | How long readers wait for the artifacts to exist |
| `agent_ipc.host` / `agent_ipc.port` | `localhost` / `0` (disabled) | Dedicated mTLS config server |
| `agent_ipc.use_socket` / `agent_ipc.socket_path` | `false` / `${run_path}/agent_ipc.socket` | Unix-socket transport for the config server |
| `agent_ipc.config_refresh_interval` | 0 (disabled) | configsync poll period in satellites |
| `agent_ipc.grpc_max_message_size` | 128 MiB | gRPC hard message cap |
| `remote_agent.registry.enabled` | true | Remote-agent registry on the core agent |
| `vsock_addr` | — | Serve the command API over vsock instead of TCP |
| `server_timeout` | 30 s | Read/write timeout on the API servers |

All keys map to `DD_*` environment variables (for example `DD_CMD_PORT`).

## Deployment-mode differences

1. **Host installs**: everything above localhost TCP + local sockets; configsync off by default (satellites re-resolve secrets themselves or rely on shared files).
1. **Kubernetes DaemonSet**: one container per process; the Helm chart enables `agent_ipc` config sync between containers, sets `health_port: 5555` for probes, and points satellites at the core agent container over pod-local TCP. Node agent to DCA traffic crosses the network on 5005 with the DCA token.
1. **Fargate**: agent and trace-agent as task sidecars; same localhost topology inside the task, no system-probe.
1. **Windows**: named pipes replace UDS for system-probe and the installer; everything else is the same TCP layout.

## Gotchas

1. **Port collisions on 5000/5001 are common** — Flask dev servers default to 5000, macOS AirPlay Receiver listens on 5000, and anything squatting on 5001 prevents agent startup. All ports are configurable but most runbooks assume defaults.
1. **gRPC on 5001 requires client certificates**, not just the bearer token. External clients written against the pre-mTLS behavior break against the `AgentSecure` interceptors.
1. **The auth fingerprint log line is the debugging tool** for cross-process auth failures: compare `successfully loaded the IPC auth primitives (fingerprint: ...)` across process logs; different fingerprints mean a process loaded stale or foreign artifacts.
1. **system-probe's socket has no token auth.** Its security boundary is file permissions / pipe DACLs only. Never widen the socket's mode as a "fix" for connectivity issues.
1. **`cmd_port` vs `agent_ipc.port` vs `ipc_address`** — three settings, three meanings; see the note above.
1. **Satellites block on artifact creation.** A satellite starting before the core agent has ever run waits `auth_init_timeout` (30 s) and then fails; the fix is to start (or restart) the core agent, not to hand-create tokens.
1. **DCA external paths reject the pod-local token** — a node agent misconfigured with the IPC token instead of `cluster_agent.auth_token` gets 401s only on external routes, which can look like partial connectivity. The DCA also enforces TLS 1.3 unless `cluster_agent.allow_legacy_tls` is set.
