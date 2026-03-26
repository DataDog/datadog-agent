> **TL;DR:** `pkg/diagnose` provides the concrete agent self-diagnostic suites — connectivity checks to Datadog endpoints, port-conflict detection, and Windows Firewall scanning — that are run by `datadog-agent diagnose` and attached to flares.

# pkg/diagnose — Agent Self-Diagnostics Framework

## Purpose

`pkg/diagnose` implements the **agent self-diagnostics** system. It provides pre-built
diagnosis suites that check connectivity to Datadog endpoints, validate port assignments,
and detect firewall rules that may block network traffic.

The results of these suites are surfaced via two user-facing commands:

- `datadog-agent diagnose` — runs all registered suites and prints a human-readable report.
- `datadog-agent flare` — runs the suites and attaches a `diagnose.log` to the flare archive.

The framework itself (suite registration, `Diagnosis` type, status codes) lives in
`comp/core/diagnose/def`. This package (`pkg/diagnose`) contains the concrete
implementations of individual diagnosis suites.

## Key elements

### Key types

#### `comp/core/diagnose/def` framework types (used throughout)

| Type | Description |
|------|-------------|
| `diagnose.Config` | Options passed to each suite: `Verbose bool`, `ForceLocal bool`. |
| `diagnose.Diagnosis` | Result of a single check: `Status`, `Name`, `Diagnosis` (human text), `Remediation`, `RawError`. |
| `diagnose.DiagnosisSuccess` / `DiagnosisFail` / `DiagnosisWarning` / `DiagnosisUnexpectedError` | Status constants. |
| `diagnose.MetadataAvailCatalog` | Global map of named availability check functions used by `DiagnoseMetadataAutodiscoveryConnectivity`. |

### Key functions

#### Sub-packages

#### `connectivity/`

Checks network reachability of Datadog intake endpoints by sending real HTTP/HTTPS requests.

Key functions:

| Function | Description |
|----------|-------------|
| `Diagnose(diagCfg, log) []diagnose.Diagnosis` | Main entry point. Initializes domain resolvers from `datadog.yaml`, then calls `checkLogsEndpoint` and one `checkEndpoint` per (domain, API key, endpoint) combination. |
| `DiagnoseMetadataAutodiscoveryConnectivity() []diagnose.Diagnosis` | Iterates `diagnose.MetadataAvailCatalog` and reports availability of each auto-discovered environment (e.g. Kubernetes, Docker). |
| `URLhasFQDN(url string) bool` | Returns true if the URL's hostname ends with `.` (fully-qualified). |
| `URLwithPQDN(url string) (string, error)` | Strips the trailing dot to produce a partially-qualified domain name. Used to diagnose FQDN vs PQDN connectivity failures. |

Checked endpoints (from `getEndpointsInfo`):

- v1: series, check runs, intake, validate, metadata
- v2: series, sketch series
- Flare endpoint (HEAD request)
- Logs endpoint (HTTP or TCP depending on `logs_config.force_use_tcp`)

The `connDiagnostician` uses `net/http/httptrace` to capture DNS resolution, connection
establishment, and TLS handshake events. These traces appear in the diagnosis output when
`diagCfg.Verbose` is true or when the check fails.

#### `firewallscanner/`

Checks whether Windows Firewall rules block UDP ports required by SNMP traps and Netflow
listeners. Currently only implemented on Windows; returns an empty slice on other platforms.

| Function | Description |
|----------|-------------|
| `Diagnose(config) []diagnose.Diagnosis` | Collects rules to check from `network_devices.snmp_traps` and `network_devices.netflow.listeners`, then delegates to the platform-specific `firewallScanner`. |

The `firewallScanner` interface has a single method:
```go
DiagnoseBlockingRules(rulesToCheck sourcesByRule) []diagnose.Diagnosis
```

The Windows implementation (`windowsFirewallScanner`) queries the Windows Firewall API to
find rules that block the required ports.

#### `ports/`

Cross-platform check that validates all configured ports in `datadog.yaml`.

| Function | Description |
|----------|-------------|
| `DiagnosePortSuite() []diagnose.Diagnosis` | Scans all config keys ending in `port`, `port_*`, or `*_port`. For each non-zero value it checks whether the port is in use via `pkg/util/port.GetUsedPorts()`. Reports success if the port is used by an agent process, a warning if it is used by an unknown process (PID = 0), and a failure if it is used by a non-agent process. |

Known agent process names checked by `isAgentProcess`:
`agent`, `trace-agent`, `trace-loader`, `process-agent`, `system-probe`, `security-agent`,
`agent-data-plane`, `privateactionrunner`.

Platform-specific port listing:
- `ports.go` — common logic.
- `ports_windows.go` — Windows implementation using `iphlpapi.dll`.
- `ports_others.go` — Linux/macOS stub calling `netstat`/`ss`.

### Configuration and build flags

No dedicated config section for this package. Sub-packages read standard agent keys (see below). The `firewallscanner` sub-package implements Windows-only logic with a no-op stub on other platforms.

#### Configuration keys

No dedicated config section. `connectivity/` reads standard agent config keys:
- `logs_enabled`, `logs_config.*`
- `convert_dd_site_fqdn.enabled`
- Multi-endpoint configuration via `utils.GetMultipleEndpoints`.

`firewallscanner/` reads:
- `network_devices.snmp_traps.enabled`, `network_devices.snmp_traps.port`
- `network_devices.netflow.enabled`, `network_devices.netflow.listeners`

## Usage

### Running diagnostics from the CLI

```bash
# Run all suites
datadog-agent diagnose all

# Connectivity only, verbose
datadog-agent diagnose show-endpoints --verbose

# Specific suite
datadog-agent diagnose --include connectivity-datadog-core-endpoints
```

### Registering a new diagnosis suite

Suites are registered against the global catalog in `comp/core/diagnose/def` via
`diagnose.GetCatalog().Register`. The suite name must be one of the pre-declared constants
in `AllSuites` (see `comp/core/diagnose` for the full list). The `pkg/diagnose` packages
expose functions that the agent wires in at startup inside `cmd/agent/subcommands/run/command.go`.

```go
package mycheck

import (
    diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func RegisterSuites(catalog diagnose.Catalog) {
    catalog.Register(diagnose.CoreEndpointsConnectivity, runDiagnose)
}

func runDiagnose(cfg diagnose.Config) []diagnose.Diagnosis {
    // ... perform checks ...
    return []diagnose.Diagnosis{
        {
            Status:    diagnose.DiagnosisSuccess,
            Name:      "My check",
            Diagnosis: "Everything is fine",
        },
    }
}
```

`diagnose.Config` carries `Verbose bool`, `Include []string`, and `Exclude []string`.
`Include`/`Exclude` are compiled as regular expressions matched against suite names.

By default, registered suites run in the context of the running agent service. If the
agent is unreachable, the CLI falls back to `RunLocalSuite` which runs suites directly
in the CLI process (see `comp/core/diagnose/local`).

### Execution context

The `Diagnose` function in `connectivity/` does not use a pre-existing HTTP client from the
agent's forwarder — it creates its own `http.Client` instance using
`pkg/util/http.CreateHTTPTransport` to imitate the forwarder behavior while remaining
callable from both the running agent and the CLI. This means it picks up all standard
transport settings (`skip_ssl_validation`, `min_tls_version`, proxy configuration, etc.)
from `datadog.yaml`. See [pkg/util/http](../util/http.md) for the full transport API.

The multi-endpoint resolution in `connectivity/` calls `utils.GetMultipleEndpoints`, which
reads `api_key`, `dd_url`, and the additional-endpoints map from the agent config. These
keys are defined in `pkg/config/setup` and are read via `model.Reader`. See
[pkg/config](../config/config.md) for how config is loaded and the source priority model.

### Testing

```bash
dda inv test --targets=./pkg/diagnose/...
```

Integration testing connectivity checks requires a network-accessible Datadog site and a
valid API key configured in `datadog.yaml`.

## Cross-references

| Topic | See also |
|-------|----------|
| FX component, global suite catalog, `Diagnosis`/`Result` types, `POST /diagnose` IPC endpoint | [comp/core/diagnose](../../comp/core/diagnose.md) |
| HTTP transport used by connectivity checks (`CreateHTTPTransport`, proxy config, TLS settings) | [pkg/util/http](../util/http.md) |
| Agent config keys read by suite implementations (`dd_url`, `api_key`, `logs_config.*`, etc.) | [pkg/config](../config/config.md) |
| Flare integration — diagnose output is attached as `diagnose.log` to every flare | [comp/core/flare](../../comp/core/flare.md) |
