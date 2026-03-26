# comp/trace-telemetry

## Purpose

The `trace-telemetry` component exposes two Prometheus-style gauges that reflect the operational state of the trace-agent process as seen from the main agent:

- `trace.enabled` — `1.0` when the trace-agent process is reachable, `0.0` otherwise.
- `trace.working` — `1.0` when the trace-agent has sent trace or stats data since it last started, `0.0` otherwise.

The component polls the trace-agent's `/debug/vars` expvar endpoint every 45 seconds (the trace-agent resets expvars every minute, so polling more frequently avoids missed data). Once the `working` state has been confirmed, subsequent polls are skipped to avoid unnecessary HTTP calls.

## Key elements

### `comp/trace-telemetry/def`

| Symbol | Description |
|--------|-------------|
| `Component` | Empty marker interface; the component runs entirely as a side effect (background goroutine). |

### `comp/trace-telemetry/impl`

| Symbol | Description |
|--------|-------------|
| `Requires` | fx dependency struct: `config.Component`, `ipc.HTTPClient`, `log.Component`, `telemetry.Component`, `compdef.Lifecycle`. |
| `Provides` | fx output struct containing `tracetelemetry.Component`. |
| `NewComponent(reqs Requires) (Provides, error)` | Constructor; registers `OnStart`/`OnStop` lifecycle hooks. |
| `tracetelemetryImpl` | Private implementation. Holds two `telemetry.Gauge` values (`enabled`, `working`) and two `atomic.Bool` fields (`running`, `sending`) that are updated by `updateState()`. |
| `traceAgentExpvars` | JSON-deserialization struct matching the trace-agent's expvar output (`trace_writer.bytes`, `stats_writer.bytes`). |

Key internal methods:

- `Start()` — launches the polling goroutine with a `time.Ticker` (45 s) and a cancellable context.
- `Stop()` — cancels the context, which terminates the goroutine.
- `updateState()` — fetches `https://localhost:<apm_config.debug.port>/debug/vars`, marks `running = true` on success, marks `sending = true` if any bytes have been written. The method is a no-op once `sending` is `true` (state is sticky).

### `comp/trace-telemetry/fx`

Provides the fx `Module()`. The module forces instantiation unconditionally via `fx.Invoke`, so the background goroutine always starts when the main agent runs.

## Usage

The component is wired into the main agent binary in `cmd/agent/subcommands/run/command.go`:

```go
tracetelemetryfx.Module()
```

No other code needs to interact with the component directly; it registers its own lifecycle hooks and emits metrics autonomously. The emitted gauges appear in the agent's internal telemetry and can be scraped at the standard `/telemetry` endpoint.

### Gauge semantics

Both gauges use binary 0/1 values (not incremental counts) so they integrate
cleanly with alerting rules such as `min_over_time(trace.enabled[5m]) == 0`
meaning "trace-agent unreachable for at least 5 minutes".

The `working` state is **sticky**: once `updateState()` observes non-zero
`trace_writer.bytes` or `stats_writer.bytes` in the expvar response, the gauge
is set to `1.0` permanently for that process lifetime. This reflects the
intended semantics — "the trace-agent has successfully sent data since startup"
— rather than "the trace-agent is currently sending data".

### Configuration

The component reads `apm_config.debug.port` via `config.Component` to build the
polling URL `https://localhost:<port>/debug/vars`. The default port is `5012`
(set by `pkg/trace/config`). If `apm_config.enabled` is `false` the
trace-agent is not started, so `trace.enabled` will remain `0.0`.

### Testing

Because the component interface is an empty marker, tests that need to satisfy
the fx dependency graph without spinning up a real polling loop can register a
`tracetelemetry.Component` no-op via `fx.Provide`. The
`telemetryimpl.MockModule()` pattern from `pkg/telemetry` can be used to assert
gauge values in integration tests.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/trace/agent` | [comp/trace/agent.md](comp/trace/agent.md) | The trace-agent component whose `/debug/vars` endpoint this component polls. The expvar fields parsed here (`trace_writer.bytes`, `stats_writer.bytes`) are emitted by `pkg/trace/writer`. When `comp/trace/agent` is not wired (trace-agent disabled), this component's gauges remain `0`. |
| `pkg/telemetry` | [../pkg/telemetry.md](../pkg/telemetry.md) | Provides `telemetry.Gauge` and `telemetry.Component`. The two gauges (`trace.enabled`, `trace.working`) are registered via `telemetry.NewGauge` at construction time and are scraped by the agent's `/telemetry` Prometheus endpoint. |
| `comp/core/ipc` | [comp/core/ipc.md](comp/core/ipc.md) | Provides the `ipc.HTTPClient` used to make authenticated requests to the trace-agent debug server. The same client pattern is used by `comp/trace/status` for the status page — both components poll the same `/debug/vars` endpoint independently. |
