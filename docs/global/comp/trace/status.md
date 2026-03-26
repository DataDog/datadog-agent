# comp/trace/status

**Team:** agent-apm
**Import path:** `github.com/DataDog/datadog-agent/comp/trace/status` (component marker)
**fx module:** `github.com/DataDog/datadog-agent/comp/trace/status/statusimpl`

## Purpose

`comp/trace/status` plugs the trace-agent into the Agent's unified status
framework (`comp/core/status`). It contributes an `InformationProvider` that
fetches live stats from the trace-agent's debug server and renders them in the
`agent status` output under the **APM Agent** section.

The component has no public methods of its own (the `Component` interface in
`component.go` is empty); its value is entirely in the side-effect of
registering the status provider at fx startup.

## Key elements

### Component interface (`comp/trace/status/component.go`)

```go
type Component interface{}
```

The interface is intentionally empty. The component is included solely for its
fx output (see below).

### `statusimpl.Module()` (`comp/trace/status/statusimpl`)

```go
func Module() fxutil.Module {
    return fxutil.Component(fx.Provide(newStatus))
}
```

`newStatus` returns a `provides` struct with a single `fx.Out` field:

```go
type provides struct {
    fx.Out
    StatusProvider status.InformationProvider
}
```

This is the standard pattern for contributing to the multi-valued
`status.InformationProvider` group consumed by `comp/core/status`.

### `statusProvider`

| Method | Behaviour |
|---|---|
| `Name()` | Returns `"APM Agent"` |
| `Section()` | Returns `"APM Agent"` |
| `JSON(bool, map[string]interface{})` | Fetches `/debug/vars` from `https://localhost:<apm_config.debug.port>` and populates `stats["apmStats"]`. |
| `Text(bool, io.Writer)` | Renders `traceagent.tmpl` with the same data. |
| `HTML(bool, io.Writer)` | Renders `traceagentHTML.tmpl`. |

Data is fetched via `ipc.HTTPClient` (an authenticated TLS client) on each
`status` call. If the trace-agent is not running or the port is unreachable,
the provider returns a map containing `error` and `port` keys, which the
template renders as `Status: Not running or unreachable on localhost:<port>`.

### Status template fields

The text template (`statusimpl/status_templates/traceagent.tmpl`) displays:

- Process info: PID, uptime, memory allocation, hostname, receiver address,
  backend endpoints.
- **Receiver (previous minute)**: traces/spans/bytes per tracer language and
  version; sampling warnings.
- Sampling rates: default priority rate or probabilistic sampler percentage.
- **Writer (previous minute)**: trace/stats payloads and bytes; API error
  warnings.

### Dependencies

| Dep | Purpose |
|---|---|
| `comp/core/config.Component` | Reads `apm_config.debug.port` to construct the stats URL. |
| `ipc.HTTPClient` | Makes the authenticated HTTPS request to the trace-agent debug server. |

## Usage

Include `statusimpl.Module()` in the fx app to have the APM section appear in
`agent status`. The core agent does this as part of the trace bundle:

```go
// comp/trace/bundle.go (example)
tracestatus.Module(),
```

No code needs to interact with the `Component` interface directly. The
framework calls `JSON`, `Text`, and `HTML` on the provider when assembling a
status response.

### Template rendering

The text and HTML templates live under
`comp/trace/status/statusimpl/status_templates/`. To add new fields to the
status output, extend `statusProvider.JSON` to populate additional keys in the
stats map, then reference those keys in both `traceagent.tmpl` and
`traceagentHTML.tmpl`. Use the template helpers from `comp/core/status`'s
`TextFmap()` / `HTMLFmap()` (e.g. `humanize`, `formatUnixTime`) for consistent
formatting with other sections.

### Error handling

When `JSON` cannot reach the trace-agent debug endpoint (network error, agent
not running) it populates the map with two keys:

```
stats["apmStats"] = map[string]interface{}{
    "error": "<error message>",
    "port":  <apm_config.debug.port>,
}
```

Both templates detect the `error` key and render `Status: Not running or
unreachable on localhost:<port>` rather than panicking on missing fields.

---

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/trace/agent` | [agent.md](agent.md) | Owns the trace pipeline whose `/debug/vars` endpoint this component polls. The debug port read here (`apm_config.debug.port`) is the same server started by `comp/trace/agent` on startup. |
| `comp/core/status` | [../core/status.md](../core/status.md) | The status framework that collects all `InformationProvider` registrations and exposes them via `/status`, `/{component}/status`, and the flare `status.log`. This component contributes to that framework via the `group:"status"` fx value group. |
| `pkg/trace` | [../../pkg/trace/trace.md](../../pkg/trace/trace.md) | The core trace pipeline. The `/debug/vars` fields parsed by `JSON` (e.g. `trace_writer.bytes`, `stats_writer.bytes`, `receiver.*`) are expvars emitted by components in `pkg/trace/writer`, `pkg/trace/api`, and `pkg/trace/sampler`. |
| `comp/core/ipc` | [../core/ipc.md](../core/ipc.md) | Provides the `HTTPClient` used to fetch `/debug/vars`. The client adds bearer-token auth and mutual-TLS to the request, so the trace-agent debug server must be started with matching TLS config (handled by `comp/trace/agent`). |
