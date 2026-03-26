> **TL;DR:** Integrates DogStatsD runtime metrics into the agent's unified status page by implementing `status.InformationProvider`, rendering expvar counters as text and HTML under a "DogStatsD" section — or registering a no-op when DogStatsD is handled by the Agent Data Plane.

# comp/dogstatsd/status

**Team:** agent-metric-pipelines

## Purpose

This component integrates DogStatsD runtime metrics into the agent's unified status page. It implements the `status.InformationProvider` interface so that `agent status` (text output), the web UI (HTML output), and the status API (JSON output) all include a "DogStatsD" section when DogStatsD is running internally.

When DogStatsD is handled by the Agent Data Plane, the component registers a no-op provider so the section is omitted.

## Key elements

### Key interfaces

`comp/dogstatsd/status/component.go`

```go
type Component interface{}
```

The interface carries no exported methods. All behavior is expressed through the `status.InformationProvider` that the implementation registers with the core status framework via fx output tag.

### Key types

#### `statusProvider`

`comp/dogstatsd/status/statusimpl/status.go`

The concrete type that implements `status.Provider`:

| Method | Description |
|--------|-------------|
| `Name() string` | Returns `"DogStatsD"` |
| `Section() string` | Returns `"DogStatsD"` |
| `JSON(verbose bool, stats map[string]interface{}) error` | Merges expvar data into the status map |
| `Text(verbose bool, buffer io.Writer) error` | Renders `dogstatsd.tmpl` |
| `HTML(verbose bool, buffer io.Writer) error` | Renders `dogstatsdHTML.tmpl` |

### Configuration and build flags

No dedicated config keys. Conditional registration is controlled by `dsdconfig.EnabledInternal()`, which reads `use_dogstatsd` and the ADP flags (see [config.md](config.md)).

#### Data source: expvar

The status is populated from three expvar keys published by the DogStatsD server:

| expvar key | Merged as |
|------------|-----------|
| `dogstatsd` | top-level keys (e.g. `PacketsLastSec`) |
| `dogstatsd-uds` | prefixed with `Uds` |
| `dogstatsd-udp` | prefixed with `Udp` |

All three are merged into a single `dogstatsdStats` map passed to both templates.

#### Templates

Embedded in the binary via `//go:embed status_templates`:

- `status_templates/dogstatsd.tmpl` — plain-text template, iterates over `dogstatsdStats` and prints key/value pairs. Appends a tip about `dogstatsd_metrics_stats_enable`.
- `status_templates/dogstatsdHTML.tmpl` — HTML variant for the web UI.

#### Conditional registration

`newStatusProvider` creates a `statusProvider` only when `dsdconfig.EnabledInternal()` is `true`. It then wraps the provider (or `nil`) in `status.NewInformationProvider`. The core status framework ignores nil providers, so no DogStatsD section appears when the feature is disabled or handed off to ADP.

## Usage

### Wire-up

The component is registered via `statusimpl.Module()` which is included in the DogStatsD bundle. It emits a `status.InformationProvider` as an fx output, which the core status aggregator collects automatically.

Dependencies:

- `comp/core/config` — used to construct a `dsdconfig.Config` to check `EnabledInternal()`.
- `comp/core/status` — provides `status.Provider`, `status.InformationProvider`, `status.RenderText`, and `status.RenderHTML`.
- `comp/dogstatsd/config` — routing decision (see [config.md](config.md)).

### Adding fields to the status page

The status data comes from expvar variables registered by the DogStatsD server. To surface a new metric:

1. Register a new `expvar.Int` or `expvar.Map` in the server under the `dogstatsd`, `dogstatsd-uds`, or `dogstatsd-udp` namespace.
2. Update `dogstatsd.tmpl` and `dogstatsdHTML.tmpl` if you want custom formatting rather than the default `formatTitle`/`humanize` rendering.
