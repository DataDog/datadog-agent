# comp/core/status

## Purpose

`comp/core/status` is the fx component that powers the agent status page — the output of `datadog-agent status`. It acts as an aggregation hub: individual components contribute status data by registering as **providers**, and the component assembles them into a single response in text, JSON, or HTML format.

The component also exposes three HTTP endpoints on the agent IPC API (`/status`, `/{component}/status`, `/status/sections`), and automatically adds a `status.log` to every flare.

---

## Package Layout

| Path | Role |
|---|---|
| `comp/core/status` (root) | Public interface, `Provider`/`HeaderProvider` interfaces, FX helper types |
| `comp/core/status/statusimpl` | Implementation: provider aggregation, rendering, API endpoint handlers, flare integration |

---

## Key Elements

### Component interface

```go
type Component interface {
    GetStatus(format string, verbose bool, excludeSection ...string) ([]byte, error)
    GetStatusBySections(sections []string, format string, verbose bool) ([]byte, error)
    GetSections() []string
}
```

`format` is one of `"json"`, `"text"`, or `"html"`. `GetSections()` returns `["header", <sorted section names>...]`.

### Provider interfaces

Providers are the extension point. Any component that wants to appear in the status output implements one of these interfaces and registers it via FX:

```go
type Provider interface {
    Name() string    // used for alphabetical sort within a section
    Section() string // groups providers under a header in text output
    JSON(verbose bool, stats map[string]interface{}) error
    Text(verbose bool, buffer io.Writer) error
    HTML(verbose bool, buffer io.Writer) error
}

type HeaderProvider interface {
    Index() int  // controls display order in the header block
    Name() string
    JSON(verbose bool, stats map[string]interface{}) error
    Text(verbose bool, buffer io.Writer) error
    HTML(verbose bool, buffer io.Writer) error
}
```

`Provider` contributes to a named section (e.g., `"collector"`, `"dogstatsd"`). `HeaderProvider` contributes to the top-level header block shown before all sections.

### FX registration types

The component uses [FX value groups](https://pkg.go.dev/go.uber.org/fx#hdr-Value_Groups) to collect providers:

```go
// Register a regular section provider (group:"status")
type InformationProvider struct {
    fx.Out
    Provider Provider `group:"status"`
}

// Register a header provider (group:"header_status")
type HeaderInformationProvider struct {
    fx.Out
    Provider HeaderProvider `group:"header_status"`
}
```

Helper constructors:

```go
status.NewInformationProvider(myProvider)       // returns InformationProvider
status.NewHeaderInformationProvider(myProvider) // returns HeaderInformationProvider
```

### Params

```go
type Params struct {
    PythonVersionGetFunc func() string
}
```

Injected via `fx.Supply(params)` at app construction time (see `statusimpl.Module()`).

### Constants

`CollectorSection = "collector"` — the collector section is always rendered first regardless of alphabetical order.

### Render helpers (`render_helpers.go`)

`HTMLFmap()` and `TextFmap()` return template function maps for use in provider templates. These include helpers like `humanize`, `formatUnixTime`, `formatFloat`, `printDashes`, and `doNotEscape`.

`PrintDashes(text, sep string) string` — generates a separator line matching the length of `text`.

### API endpoints (registered by the implementation)

| Method | Path | Description |
|---|---|---|
| GET | `/status` | Full agent status in the format requested via query string |
| GET | `/{component}/status` | Status for a single section |
| GET | `/status/sections` | JSON list of available section names |

### Flare integration

`statusimpl` registers itself as a flare provider. Every flare automatically includes `status.log` (verbose text output of the full status).

---

## Usage

### Wiring the component into an FX app

```go
import (
    "github.com/DataDog/datadog-agent/comp/core/status"
    statusimpl "github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
)

fx.Options(
    statusimpl.Module(),
    fx.Supply(status.Params{
        PythonVersionGetFunc: python.GetPythonVersion,
    }),
)
```

### Writing a new status provider

1. Implement `status.Provider` (or `status.HeaderProvider`) on your component's struct.
2. Return a `status.InformationProvider` from your component's `Provides` struct:

```go
type provides struct {
    fx.Out
    Status status.InformationProvider
}

func newMyComponent(deps dependencies) provides {
    p := &myStatusProvider{}
    return provides{
        Status: status.NewInformationProvider(p),
    }
}

func (p *myStatusProvider) Name() string    { return "My Component" }
func (p *myStatusProvider) Section() string { return "my-section" }
func (p *myStatusProvider) JSON(verbose bool, stats map[string]interface{}) error { ... }
func (p *myStatusProvider) Text(verbose bool, w io.Writer) error { ... }
func (p *myStatusProvider) HTML(verbose bool, w io.Writer) error { ... }
```

### Real-world examples

- `comp/dogstatsd/status/statusimpl` — DogStatsD section provider; conditionally active only when DogStatsD is enabled.
- `comp/forwarder/defaultforwarder` — Forwarder section with queue depth and transaction metrics.
- `comp/trace/status/statusimpl` — Trace-agent section provider.
- `cmd/agent/subcommands/run/command.go` — Wires `statusimpl.Module()` and supplies `Params` for the core agent.

### Consuming status from the CLI

```go
// Inject status.Component and call:
output, err := statusComp.GetStatus("text", verbose)
// or for a specific section:
output, err := statusComp.GetStatusBySections([]string{"collector"}, "json", verbose)
```

---

## Related packages

- `pkg/status` — contains concrete `Provider` implementations for each logical area (collector, endpoints, JMX, system-probe, etc.), the `pkg/status/health` liveness/readiness catalog (surfaced via `/agent/status/health`), and the `pkg/status/render` helpers used by the cluster agent. See [pkg/status docs](../../pkg/status/status.md).
- `comp/core/flare` — `statusimpl` automatically registers a flare provider that adds `status.log` (verbose text of the full status) to every flare. See [comp/core/flare docs](flare.md).
- `comp/core/diagnose` — `diagnoseimpl` similarly adds `diagnose.log` to every flare; it is a sibling "health" surface alongside status. See [comp/core/diagnose docs](diagnose.md).
- `comp/core/config` — status providers typically depend on this to read config keys when rendering their sections. See [comp/core/config docs](config.md).
