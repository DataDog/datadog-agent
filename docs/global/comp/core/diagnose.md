> **TL;DR:** `comp/core/diagnose` runs focused health-check suites (connectivity, port conflicts, firewall, check execution) against a live agent and reports pass/fail/warning results via CLI, a `POST /diagnose` IPC endpoint, and a `diagnose.log` entry in every flare.

# comp/core/diagnose

## Purpose

`comp/core/diagnose` is the fx component that runs **diagnostic suites** against a live or locally-reachable agent. Each suite performs a focused health check (connectivity to Datadog endpoints, autodiscovery reachability, port conflicts, firewall scanning, check execution, health platform issues) and reports pass/fail/warning results.

The output of `datadog-agent diagnose` feeds this component. Results are available in text or JSON format, are exposed via a `POST /diagnose` IPC endpoint, and are automatically included in every flare as `diagnose.log`.

---

## Package Layout

| Path | Role |
|---|---|
| `comp/core/diagnose/def` | Public interface, all shared types (`Component`, `Diagnosis`, `Result`, `Catalog`, `Config`, `Status`), global suite catalog |
| `comp/core/diagnose/impl` | Implementation: suite execution, filtering, HTTP handler, flare provider |
| `comp/core/diagnose/fx` | `Module()` — FX wiring |
| `comp/core/diagnose/format` | `Text` and `JSON` output formatters |
| `comp/core/diagnose/mock` | Test mock |
| `comp/core/diagnose/local` | Helpers for running diagnose directly in the CLI process |

---

## Key Elements

### Key interfaces

#### Component interface

```go
type Component interface {
    RunSuites(format string, verbose bool) ([]byte, error)
    RunSuite(suite string, format string, verbose bool) ([]byte, error)
    RunLocalSuite(suites Suites, config Config) (*Result, error)
}
```

- `RunSuites` — runs every registered suite in the global catalog.
- `RunSuite` — runs a single named suite.
- `RunLocalSuite` — runs a caller-supplied set of suite functions directly (used by the CLI process when the agent is unreachable).

`format` is `"text"` or `"json"`.

### Key types

#### Available suites

Suites must be declared in `AllSuites` before registration:

| Constant | Suite name | What it checks |
|---|---|---|
| `CheckDatadog` | `check-datadog` | Integration check execution |
| `AutodiscoveryConnectivity` | `connectivity-datadog-autodiscovery` | Autodiscovery service reachability |
| `CoreEndpointsConnectivity` | `connectivity-datadog-core-endpoints` | Datadog intake/API endpoint connectivity |
| `EventPlatformConnectivity` | `connectivity-datadog-event-platform` | Event Platform endpoint connectivity |
| `PortConflict` | `port-conflict` | Ports used by the agent not already bound |
| `FirewallScan` | `firewall-scan` | Outbound firewall rules |
| `HealthPlatformIssues` | `health-issues` | Health platform status |

#### Global Catalog

Suites are registered into a process-global singleton via `GetCatalog().Register`:

```go
catalog := diagnose.GetCatalog()
catalog.Register(diagnose.CoreEndpointsConnectivity, func(cfg diagnose.Config) []diagnose.Diagnosis {
    return runConnectivityChecks(cfg)
})
```

`Register` panics if the suite name is not in `AllSuites`. Only one function per suite name is allowed (silently ignored on duplicate for the metadata catalog `RegisterMetadataAvail`).

#### Config

```go
type Config struct {
    Verbose bool
    Include []string // regexp patterns — run only matching suite names
    Exclude []string // regexp patterns — skip matching suite names
}
```

`Include`/`Exclude` are compiled as regular expressions and matched against suite names. If both are set, a suite must match `Include` AND not match `Exclude`.

#### Diagnosis and Result types

```go
type Diagnosis struct {
    Status      Status            // DiagnosisSuccess/Fail/Warning/UnexpectedError
    Name        string            // Required: short identifier
    Diagnosis   string            // Required: human-readable result message
    Category    string            // Optional: grouping label
    Description string            // Optional: what is being tested (shown in verbose mode)
    Remediation string            // Optional: how to fix a failure
    RawError    string            // Optional: underlying error string
    Metadata    map[string]string // Optional: additional key-value pairs
}

type Result struct {
    Runs    []Diagnoses // per-suite results
    Summary Counters    // Total / Success / Fail / Warnings / UnexpectedErr
}
```

`Status.ToString(colors bool)` returns `"PASS"` / `"FAIL"` / `"WARNING"` / `"UNEXPECTED ERROR"` (with ANSI color codes when `colors=true`).

`Diagnosis.MarshalJSON` adds a `connectivity_result` string field alongside the numeric `result` field for cross-language consumers (Python checks, rtloader).

#### Status constants

```go
DiagnosisSuccess         Status = 0
DiagnosisFail            Status = 1
DiagnosisWarning         Status = 2
DiagnosisUnexpectedError        = 3
```

The numeric values are shared with Python (`integrations-core`) and the rtloader C header. Do not renumber them.

### Configuration and build flags

#### FX Module

```go
// comp/core/diagnose/fx
func Module() fxutil.Module
```

Wires `diagnoseimpl.NewComponent` and exposes an optional `diagnose.Component` binding (the component is optional so agents that don't need diagnose can omit it without breaking the FX graph).

#### What the implementation provides

`diagnoseimpl.NewComponent` returns a `Provides` struct with:
- `Comp` — the `diagnose.Component` implementation.
- `APIDiagnose` — a `POST /diagnose` endpoint that deserializes a `Config` from the request body and streams back `Result` as JSON.
- `FlareProvider` — a `flaretypes.Provider` that adds `diagnose.log` (verbose text) to every flare.

---

## Usage

### Wiring into an FX app

```go
import diagnosefx "github.com/DataDog/datadog-agent/comp/core/diagnose/fx"

fx.Options(
    diagnosefx.Module(),
)
```

The diagnose component has no required FX dependencies (`Requires` is empty), so it can be added to any app.

### Registering a suite

Suite registration happens at application startup — typically inside the `start` command's initialization function:

```go
import diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"

diagnoseCatalog := diagnose.GetCatalog()
diagnoseCatalog.Register(diagnose.CoreEndpointsConnectivity, func(cfg diagnose.Config) []diagnose.Diagnosis {
    return connectivity.RunCoreEndpointChecks(cfg)
})
```

The function signature is `func(Config) []Diagnosis`. Suites must return at least one `Diagnosis`; an empty slice means the suite produced no output and it is omitted from results. Any invalid `Diagnosis` (missing `Name` or `Diagnosis` field, out-of-range `Status`) is automatically converted to `DiagnosisUnexpectedError` by the runner.

### Running suites from the CLI

The CLI calls the agent's `POST /diagnose` endpoint with a serialized `Config`. If the agent is unreachable it falls back to `RunLocalSuite` with suites constructed locally. See `cmd/agent/subcommands/diagnose/command.go` for the full flow.

### Real-world registrations (core agent)

All seven built-in suites are registered in `cmd/agent/subcommands/run/command.go` within the `fx.Invoke` that runs after the FX graph is built:

```go
diagnosecatalog.Register(diagnose.CheckDatadog, ...)
diagnosecatalog.Register(diagnose.PortConflict, ...)
diagnosecatalog.Register(diagnose.EventPlatformConnectivity, ...)
diagnosecatalog.Register(diagnose.AutodiscoveryConnectivity, ...)
diagnosecatalog.Register(diagnose.CoreEndpointsConnectivity, ...)
diagnosecatalog.Register(diagnose.FirewallScan, ...)
diagnosecatalog.Register(diagnose.HealthPlatformIssues, ...)
```

The cluster agent registers its own subset in `cmd/cluster-agent/subcommands/start/command.go`.

---

## Related packages

- `pkg/diagnose` — contains the concrete suite implementations: `connectivity/` (HTTP/HTTPS endpoint reachability with `httptrace` instrumentation), `firewallscanner/` (Windows firewall rule inspection), and `ports/` (cross-platform port conflict detection). These are the functions registered into the global catalog at agent startup. See [pkg/diagnose docs](../../pkg/diagnose/diagnose.md).
- `comp/core/flare` — `diagnoseimpl` self-registers a flare provider that runs all suites in verbose mode and attaches the output as `diagnose.log` to every flare archive. See [comp/core/flare docs](flare.md).
- `comp/core/config` — suite implementations read agent configuration (API endpoints, port settings, network device config, etc.) to determine what to check. See [comp/core/config docs](config.md).
