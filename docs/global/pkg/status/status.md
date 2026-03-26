# pkg/status

## Purpose

`pkg/status` provides two distinct but related capabilities:

1. **Agent status page data** — A collection of sub-packages that gather runtime information
   (check runner stats, JMX state, endpoint configuration, cluster-check assignments, etc.)
   and expose it as structured data for the agent's status page. Each sub-package implements
   the `comp/core/status.Provider` interface, which allows the status component to aggregate
   data from many sources and render it as JSON, human-readable text, or HTML.

2. **Internal health checks** (`pkg/status/health`) — A lightweight, goroutine-level
   liveness and readiness system. Agent components register themselves with a name and
   periodically drain a channel to prove they are not frozen. The health catalog is polled
   by the agent's `/agent/status/health` HTTP endpoint and the `agent health` CLI command.

The root `pkg/status` package itself is a thin helper that reads the `runner` expvar and
deserialises it into `CLCChecks` / `CLCStats` types for use by the Cluster Level Check (CLC)
runner API.

## Key elements

### Root package (`pkg/status`)

| Type | Purpose |
|------|---------|
| `CLCChecks` | Deserialised view of the `runner` expvar. Contains per-check stats and worker pool info for the CLC runner. |
| `CLCStats` | Per-check-instance stats: `AverageExecutionTime`, `MetricSamples`, `HistogramBuckets`, `Events`, `LastExecFailed`. |
| `Workers` | Worker pool summary: `Count` and per-worker `Utilization`. |
| `GetExpvarRunnerStats() (CLCChecks, error)` | Reads the `runner` expvar and returns a typed `CLCChecks` struct. |

### `pkg/status/health`

The health package manages three independent catalogs, one per probe type:

| Catalog | Registers via | Queried via | Meaning |
|---------|---------------|-------------|---------|
| `readinessAndLivenessCatalog` | `RegisterLiveness` | `GetLive` / `GetLiveNonBlocking` | Component must stay healthy for the entire lifetime of the agent |
| `readinessOnlyCatalog` | `RegisterReadiness` | `GetReady` / `GetReadyNonBlocking` (merged with liveness) | Component must be ready before traffic is accepted |
| `startupOnlyCatalog` | `RegisterStartup` | `GetStartup` / `GetStartupNonBlocking` | Component checked only until it passes once (using the `Once` option internally) |

#### Types

```go
// Handle is returned by Register*. Components keep it alive for the
// duration of their lifecycle.
type Handle struct {
    C <-chan time.Time  // must be drained at least every 15 seconds
}

func (h *Handle) Deregister() error

// Status is returned by Get* functions.
type Status struct {
    Healthy   []string
    Unhealthy []string
}
```

#### Registration and lifecycle

```go
// Register for liveness (component must stay healthy forever):
h := health.RegisterLiveness("my-component")
defer h.Deregister()

// Register for readiness only:
h := health.RegisterReadiness("my-component")
defer h.Deregister()

// Register for startup (checked until healthy once):
h := health.RegisterStartup("my-component")
defer h.Deregister()
```

Once registered, a component is considered **unhealthy** until it reads from `h.C`. The
catalog's background goroutine (`catalog.run`) ticks every 15 seconds and writes a deadline
timestamp into each component's channel (buffer size 2). If the channel is already full
(i.e. the component has not read within two tick periods, ~30 s) the component is marked
unhealthy.

#### Querying health

```go
// Blocking — may block indefinitely if a catalog lock is held:
status := health.GetLive()
status := health.GetReady()   // liveness + readiness combined
status := health.GetStartup()

// Non-blocking — guaranteed to return within 500 ms:
status, err := health.GetLiveNonBlocking()
status, err := health.GetReadyNonBlocking()
status, err := health.GetStartupNonBlocking()
```

#### `Once` option

```go
health.RegisterLiveness("one-shot-init", health.Once)
```

When `Once` is passed the component is removed from periodic checking once it has been
marked healthy for the first time. `RegisterStartup` always applies `Once` automatically.

### `pkg/status/render`

Utility package used by the cluster agent for rendering specific status sections that are
not covered by the main status component:

| Function | Template | Purpose |
|----------|----------|---------|
| `FormatHPAStatus(data []byte) (string, error)` | `custommetricsprovider.tmpl` | Renders HPA / custom metrics provider status |
| `FormatMetadataMapCLI(data []byte) (string, error)` | `metadatamapper.tmpl` | Renders metadata mapper status for the CLI |

Both functions accept raw JSON bytes (typically from an IPC call), unmarshal them into
`map[string]interface{}`, and execute the corresponding embedded Go text template.

### Domain-specific `Provider` sub-packages

Each sub-package implements `comp/core/status.Provider` and focuses on one logical area
of the status page. The pattern is uniform: implement `Name()`, `Section()`, `JSON()`,
`Text()`, `HTML()`, and optionally `PopulateStatus(stats map[string]interface{})`.

| Sub-package | Section name | Data source |
|-------------|-------------|-------------|
| `pkg/status/collector` | `"collector"` (= `status.CollectorSection`) | `runner`, `autoconfig`, `CheckScheduler`, `pyLoader`, `pythonInit`, `inventories` expvars |
| `pkg/status/endpoints` | `"endpoints"` | `config/utils.GetMultipleEndpoints`; API keys are obfuscated to last 4 chars |
| `pkg/status/jmx` | `"JMX Fetch"` | JMX state stored in `pkg/status/jmx` package-level vars |
| `pkg/status/clusteragent` | cluster agent sections | Cluster agent API responses |
| `pkg/status/systemprobe` | system-probe sections | System probe IPC API |
| `pkg/status/httpproxy` | HTTP proxy section | Proxy config |

Templates are embedded into each sub-package binary at compile time via `//go:embed
status_templates`. `comp/core/status.RenderText` and `comp/core/status.RenderHTML` are used
to execute them.

## Usage

### Registering a component for health checks

```go
import "github.com/DataDog/datadog-agent/pkg/status/health"

func (c *MyComponent) run() {
    h := health.RegisterLiveness("my-component")
    defer h.Deregister()

    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-h.C:
            // health ping — no action needed, just draining the channel
        case <-ticker.C:
            c.doWork()
        case <-c.stopChan:
            return
        }
    }
}
```

The key point: `h.C` must be included in every `select` loop iteration. Forgetting it means
the buffer fills up after two missed ticks and the component appears unhealthy.

### Exposing a new status section

1. Create a new sub-package under `pkg/status/<area>/`.
2. Implement the `comp/core/status.Provider` interface.
3. Add embedded templates under `status_templates/`.
4. Register the provider with the status component (typically via fx, in
   `comp/core/status` or the relevant agent binary's fx wiring).

```go
// Example Provider skeleton
type Provider struct{}

func (Provider) Name() string    { return "My Section" }
func (Provider) Section() string { return "my-section" }

func (Provider) JSON(_ bool, stats map[string]interface{}) error {
    PopulateStatus(stats)
    return nil
}

func (Provider) Text(_ bool, buffer io.Writer) error {
    return status.RenderText(templatesFS, "mysection.tmpl", buffer, getStatusInfo())
}

func (Provider) HTML(_ bool, buffer io.Writer) error {
    return status.RenderHTML(templatesFS, "mysectionHTML.tmpl", buffer, getStatusInfo())
}
```

### Querying health from the CLI

The `agent health` command (`pkg/cli/subcommands/health/command.go`) calls
`GET /agent/status/health`, unmarshals the response into `health.Status`, and prints
`PASS` / `FAIL` along with the list of healthy and unhealthy components.

### Reading CLC runner stats (cluster-checks)

```go
import "github.com/DataDog/datadog-agent/pkg/status"

checks, err := status.GetExpvarRunnerStats()
if err != nil {
    return err
}
for checkName, instances := range checks.Checks {
    for _, stats := range instances {
        if stats.LastExecFailed {
            log.Warnf("check %s last run failed", checkName)
        }
    }
}
```

## Architecture: how the pieces fit together

```
┌─────────────────────────────────────────────────────────────┐
│  comp/core/status (aggregator + HTTP /status endpoint)      │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Registered Providers (fx group:"status")            │   │
│  │  pkg/status/collector  pkg/status/endpoints          │   │
│  │  pkg/status/jmx        pkg/status/systemprobe  ...   │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  comp/core/healthprobe (HTTP /live, /ready, /startup)       │
│  reads → pkg/status/health catalogs                         │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  liveness catalog   readiness catalog   startup      │   │
│  │  (RegisterLiveness) (RegisterReadiness) (RegisterStartup)│
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

The status page (`comp/core/status`) and the health probes (`comp/core/healthprobe`)
are independent subsystems. The status page renders rich diagnostic output; the health
probe exposes minimal liveness/readiness signals for orchestrators.

## Related packages

- [`comp/core/status`](../../comp/core/status.md) — defines the `Provider` interface,
  `RenderText`, `RenderHTML`, the fx value groups (`group:"status"` /
  `group:"header_status"`), and the HTTP endpoints (`/status`, `/{component}/status`,
  `/status/sections`). The providers in `pkg/status/` implement this interface and are
  registered with the component via fx.
- [`comp/core/healthprobe`](../../comp/core/healthprobe.md) — starts a lightweight HTTP
  server (`/live`, `/ready`, `/startup`) that reads from `pkg/status/health`. Configuration
  keys: `health_port` and `log_all_goroutines_when_unhealthy`. Every long-running agent
  binary (core agent, system-probe, DogStatsD, cluster agent) includes this component.
- `pkg/collector/check/stats` — defines `stats.Stats`, the full check-run statistics struct
  from which `CLCStats` is populated.
- `pkg/flare` — includes health and status data in agent flares.

### Interaction between health and flares

`comp/core/flare` (via `statusimpl`) automatically adds `status.log` — the verbose text
output of the full status page — to every flare. The health catalog state is not separately
added to the flare, but unhealthy component names appear in the status output when the
status page's collector section reports check failures.
