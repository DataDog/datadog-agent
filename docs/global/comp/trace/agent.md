# comp/trace/agent

**Team:** agent-apm
**Import path:** `github.com/DataDog/datadog-agent/comp/trace/agent/def`
**fx module:** `github.com/DataDog/datadog-agent/comp/trace/agent/fx`

## Purpose

`comp/trace/agent` is the fx component that owns the trace-agent lifecycle. It
wraps `pkg/trace/agent.Agent` — the core APM pipeline — and integrates it into
the Agent's component graph. Concretely it:

- Starts and stops the trace pipeline (receiver, samplers, writers, watchdog).
- Manages Go runtime tuning (`GOMAXPROCS`, `GOMEMLIMIT`) based on
  `apm_config.max_cpu_percent` / `apm_config.max_memory`.
- Wires up the DogStatsD client used for internal trace-agent metrics.
- Exposes OTLP ingestion (used by the embedded OTel collector).
- Registers HTTP handlers on the debug server for `/config`, `/config/set`,
  and `/secret/refresh`.
- Subscribes to remote-config updates via gRPC when remote configuration is
  enabled.
- Handles `SIGINT`/`SIGTERM`/`SIGPIPE` for graceful shutdown.

The component returns `ErrAgentDisabled` (and triggers an fx shutdown) when APM
is disabled via `apm_config.enabled: false` or `DD_APM_ENABLED=false`.

## Key elements

### Interface (`comp/trace/agent/def`)

```go
type Component interface {
    // SetOTelAttributeTranslator overrides the attribute translator used
    // by the OTLP receiver. Called by the embedded OTel collector.
    SetOTelAttributeTranslator(attrstrans *attributes.Translator)

    // ReceiveOTLPSpans injects OTLP resource spans directly into the
    // trace pipeline. Returns the resolved source (hostname/container).
    ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans,
        httpHeader http.Header,
        hostFromAttributesHandler attributes.HostFromAttributesHandler,
    ) (source.Source, error)

    // SendStatsPayload forwards a pre-computed stats payload to the
    // stats writer (used by the Datadog exporter in the OTel pipeline).
    SendStatsPayload(p *pb.StatsPayload)

    // GetHTTPHandler returns the HTTP handler registered for the given
    // receiver endpoint path (e.g. "/v0.4/traces").
    GetHTTPHandler(endpoint string) http.Handler
}
```

### Params (`comp/trace/agent/impl`)

`Params` carries CLI flags forwarded from the `trace-agent run` command:

| Field | CLI flag | Description |
|---|---|---|
| `PIDFilePath` | `--pidfile` | Path for the PID file. |
| `CPUProfile` | `--cpu-profile` | Enable pprof CPU profiling. |
| `MemProfile` | `--mem-profile` | Dump heap profile on stop. |
| `DisableInternalProfiling` | — | Suppress `internal_profiling`. |

### Internal structure

`component` (unexported) embeds `*pkg/trace/agent.Agent` and adds:

- `cancel context.CancelFunc` — stops the agent goroutine.
- `wg *sync.WaitGroup` — waits for the main `Agent.Run()` goroutine.
- `config traceconfigdef.Component` — used to subscribe to API key rotations.
- `ipc ipc.Component` — provides TLS config for the debug server.

fx lifecycle hooks call `start()` / `stop()` which in turn call
`runAgentSidekicks` (info server, remote config endpoint, profiling) and
`stopAgentSidekicks` (flush profiler, flush logs).

### fx wiring

```
comp/trace/agent/fx.Module()
  └─ fx.Provide(agentimpl.NewAgent)

comp/trace/agent/fx-mock/fx.go  — provides a no-op mock for tests
```

## Usage

### Standalone trace-agent process

`cmd/trace-agent/subcommands/run/command.go` bootstraps the full fx app:

```go
fx.Provide(traceagentimpl.NewAgent),
fx.Provide(func() *agentimpl.Params { return &agentimpl.Params{...} }),
traceconfigimpl.NewComponent,
...
```

The component is not used directly after construction; fx lifecycle hooks
manage start/stop.

### Embedded in the core agent

`cmd/agent/subcommands/run/command.go` (Windows) includes
`comp/trace/agent/fx.Module()` so the trace pipeline runs in-process alongside
the core agent.

### Embedded OTel collector

`comp/otelcol/collector/impl/collector.go` and
`comp/otelcol/otlp/components/exporter/datadogexporter/` inject
`traceagent.Component` to forward OTLP spans via `ReceiveOTLPSpans` and stats
payloads via `SendStatsPayload`, bypassing the normal HTTP receiver.

### Tests

`comp/trace/agent/fx-mock/fx.go` provides a mock module. Individual unit tests
in `impl/agent_test.go` call `agentimpl.NewAgent` directly using
`fxutil.Test`.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `pkg/trace` | [../../pkg/trace/trace.md](../../pkg/trace/trace.md) | Core trace pipeline. This component embeds `*pkg/trace/agent.Agent` and drives its `Run()` goroutine via fx lifecycle hooks. All span processing, sampling, stats, and writing logic lives in `pkg/trace`. |
| `comp/trace/config` | [config.md](config.md) | Configuration bridge. This component depends on `traceconfigdef.Component` to subscribe to API key rotations (`OnUpdateAPIKey`) and to pass `cfg.Object()` (`*pkg/trace/config.AgentConfig`) into `pkg/trace/agent.NewAgent`. |
| `comp/otelcol/otlp` | [../otelcol/otlp.md](../otelcol/otlp.md) | In-process OTLP pipeline. The OTLP pipeline forwards traces to the trace-agent's internal OTLP receiver port. The OTel Datadog exporter (`datadogexporter`) calls `ReceiveOTLPSpans` and `SendStatsPayload` on this component directly, bypassing the HTTP receiver. |
| `pkg/obfuscate` | [../../pkg/obfuscate.md](../../pkg/obfuscate.md) | Sensitive-data scrubbing. `pkg/trace/agent` constructs one `*obfuscate.Obfuscator` per agent instance (from `AgentConfig.Obfuscation`). All span obfuscation (SQL, Redis, HTTP, MongoDB, credit cards) is serialized through a single goroutine that calls methods on that obfuscator. |
| `comp/remote-config/rcclient` | [../remote-config/rcclient.md](../remote-config/rcclient.md) | RC client. When `remote_configuration.enabled` is true, this component subscribes to `ProductAPMSampling` and `ProductAgentConfig` via `pkg/trace/remoteconfighandler`, enabling runtime sampling rate and obfuscation updates without restart. |
