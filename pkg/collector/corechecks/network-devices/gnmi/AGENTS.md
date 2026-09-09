# gNMI core check

Experimental NDM check that collects OpenConfig telemetry over gNMI and reports
`snmp.*` metrics, interface metadata, and topology payloads compatible with the
SNMP integration.

Enable with `network_devices.gnmi.enabled: true` in `datadog.yaml`.

## Package layout

```
pkg/collector/corechecks/network-devices/gnmi/
├── gnmi.go              # Check implementation (Configure, Run, Cancel)
├── config/              # Instance YAML parsing, profiles, metadata paths
├── client/              # gNMI subscribe client, cache, transport
├── report/              # Snapshot → metrics, metadata, topology
├── status/              # Agent status provider (when check is enabled)
├── admission/           # Check admission gate
├── integrationtests/    # Agent-level integration tests
└── internal/fakeserver/ # Test gNMI server

cmd/agent/subcommands/gnmi/   # `agent gnmi` troubleshooting CLI
cmd/agent/dist/conf.d/gnmi.d/  # Shipped config templates and profiles
```

### Data flow

1. `Configure` builds a long-lived `client.Client` and registers device status.
2. `Run` snapshots the client cache on each collection interval.
3. `report` translates cached values into Datadog metrics and metadata events.
4. The client maintains a background gNMI subscribe stream between runs.

## Configuration

| File | Purpose |
|------|---------|
| `datadog.yaml` | `network_devices.gnmi.enabled` feature flag |
| `conf.d/gnmi.d/conf.yaml` | Per-device instances (address, credentials, profile) |
| `conf.d/gnmi.d/profiles/*.yaml` | Metric paths, metadata mappings, topology paths |

Instance fields are documented in `cmd/agent/dist/conf.d/gnmi.d/conf.yaml.default`.

Profiles support `extends` for shared metadata and topology fragments (for example
`_openconfig-metadata.yaml`, `_openconfig-lldp.yaml`).

## Troubleshooting CLI

Implementation: `cmd/agent/subcommands/gnmi/`.

User-facing guide: `docs/public/how-to/test/gnmi-troubleshooting.md`.

| Command | Purpose |
|---------|---------|
| `agent gnmi subscribe` | Live stream; print cached path values |
| `agent gnmi subscribe --list-paths` | Print resolved subscription paths (no subscribe stream) |
| `agent gnmi preview-metrics` | Print metrics and tags the check would emit |

These commands do not require the check to be scheduled. They reuse
`resolveSubscribeTarget` to load instances from `conf.d/gnmi.d/conf.yaml` or CLI
flags.

### Config resolution

- `-c` / `--cfgpath`: directory containing `datadog.yaml` (sets `confd_path`
  unless overridden).
- `--conf-file`: load a specific `conf.d/gnmi.d/conf.yaml`; also sets
  `confd_path` to the parent `conf.d` so profiles resolve correctly.
- `--instance N`: pick instance index from the conf file.

### `preview-metrics` internals

`report.PreviewCollection` mirrors `Check.Run` metric emission:

- `ReportHealth` → `datadog.gnmi.*`
- `ReportMetrics` → profile `snmp.*` metrics
- `ReportDerivedMetrics` → bandwidth and memory usage (stateful across ticks)
- `ReportInterfaceStatus` → `snmp.interface.status` when inventory is ready
- `ReportMetadata` (optional via `--include-metadata`) → `network-devices-metadata`
  JSON printed to stdout

Metrics are captured by `report.CaptureSender` (a `sender.Sender` implementation)
and formatted to stdout. Metadata event platform payloads are captured the same way
and pretty-printed as JSON.

## Other Agent commands

| Command | Purpose |
|---------|---------|
| `agent check gnmi` | Run the check once via the collector |
| `agent status` | gNMI section when the feature flag is enabled |

Filter check instances with `--instance-filter` (jq syntax on instance fields).

## Tagging conventions

Aligned with SNMP NDM for UI compatibility:

| Tag / field | Source |
|-------------|--------|
| `snmp_device` | Config `address` (verbatim; hostname or IP, never DNS-resolved) |
| `snmp_host` | Telemetry hostname |
| `device_id` | `default:<address>` (same verbatim config value as `snmp_device`) |
| Device resource tag | `dd.internal.resource:ndm_device:<device_id>` |
| `integration_source:gnmi` | Always set on metrics |
| Interface resource tag | `dd.internal.resource:ndm_interface:<device_id>:<interface_name>` |

Topology remote device `dd_id` is set only when LLDP reports a parseable IP
management address (`default:<ip>`), matching SNMP's IPv4-only LLDP management-address
handling. Hostnames are kept on `ip_address` for display but do not produce a `dd_id`
guess; correlate remote devices via chassis ID instead.

Derived `snmp.memory.usage` pairs profile `snmp.memory.used` and `snmp.memory.free` entries
that share the same parent gNMI path and identical path keys (for example the same
`name=ControlA` component key on both `.../memory/utilized` and `.../memory/available`).

Device metadata fields mapped under `/components/component/` are read using the same
keyed lookup: resolve the device component (`Chassis` when present), then fetch each
configured path with that exact key set.

Interface inventory (metadata and `snmp.interface.status`) includes interfaces
discovered from profile interface metric paths (metrics tagged with `interface:`)
as well as metadata paths.

Metadata payloads use `Integration: snmp` for NDM backend compatibility.

## Development

### Build and test

```bash
dda inv agent.build --build-exclude=systemd
dda inv test --targets=./pkg/collector/corechecks/network-devices/gnmi/...
```

### Registration

- Check factory: `pkg/commonchecks/corechecks.go`
- Core check list: `tasks/core_checks.py` and `cmd/agent/dist/core_checks.bzl`
- CLI subcommand: `cmd/agent/subcommands/subcommands.go`

### Adding metrics

1. Add paths and mappings to a profile under `conf.d/gnmi.d/profiles/`.
2. For derived metrics, extend `report/derived_metrics.go`.
3. Validate with `agent gnmi preview-metrics --once`.
4. Add unit tests in `report/*_test.go`.

### Fake server

`pkg/collector/corechecks/network-devices/gnmi/internal/fakeserver` and
`test/new-e2e/tests/ndm/gnmi/` provide test targets for local and E2E testing.

## Pitfalls

- **Staleness**: `Run` filters samples older than `2 * min_collection_interval`.
  The subscribe CLI shows all cached values; preview-metrics applies the same
  staleness filter as the check.
- **Sync gate**: Interface status and metadata require a synchronized stream and
  complete interface inventory in the cache.
- **Derived bandwidth**: Needs two collection intervals with monotonic counter
  deltas; the first `preview-metrics` tick may omit bandwidth usage rates.
- **Profile paths**: Paths in YAML are normalized to start with `/` internally.
- **confd_path**: Wrong `confd_path` breaks both instance and profile loading;
  use `--conf-file` in CLI tools to avoid ambiguity during local dev.
