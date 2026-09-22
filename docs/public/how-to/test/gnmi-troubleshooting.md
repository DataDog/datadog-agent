# gNMI troubleshooting

The Agent ships CLI tools to debug the experimental gNMI core check without
running the full collector or sending data to Datadog. Use them to validate
transport, authentication, profile paths, and metric tagging before enabling
the check in production.

## Prerequisites

1. Build the Agent (see the root `AGENTS.md` workflow).
2. Create a check instance at `conf.d/gnmi.d/conf.yaml` (copy from
   `conf.d/gnmi.d/conf.yaml.default`).

The troubleshooting commands do **not** require the check to be scheduled, but
they reuse the same profiles under `conf.d/gnmi.d/profiles/`.

## Configuration paths

| Flag | Meaning |
|------|---------|
| `-c`, `--cfgpath` | Directory containing `datadog.yaml` (for example `bin/agent/dist`) |
| `--conf-file` | Path to `conf.d/gnmi.d/conf.yaml` (overrides `confd_path` for instance loading) |
| `--instance N` | Load instance `N` from the gNMI conf file (0-based index) |

`-c` must point at the **agent config directory**, not at `conf.yaml` itself.

```bash
# Correct
./bin/agent/agent gnmi subscribe --instance 0 -c bin/agent/dist

# Also correct: point directly at the gNMI conf file
./bin/agent/agent gnmi subscribe --instance 0 \
  --conf-file bin/agent/dist/conf.d/gnmi.d/conf.yaml
```

CLI flags (`--address`, `--port`, `--username`, and so on) override values loaded
from the instance config.

## Commands

### `agent gnmi subscribe`

Opens a gNMI subscribe stream and prints cached path values on stdout. Use this
to confirm the device is reachable, credentials work, and the profile subscribes
to the expected paths.

```bash
./bin/agent/agent gnmi subscribe --instance 0 -c bin/agent/dist
```

Press `Ctrl+C` to stop.

#### List subscription paths without subscribing

Resolves paths from the loaded profile and prints them. Does not open a gNMI
subscribe stream to the device.

```bash
./bin/agent/agent gnmi subscribe --instance 0 --list-paths -c bin/agent/dist
```

#### Output format

Each tick prints a status line followed by cached values:

```
--- stream=connected synchronized=true reconnect=0 samples=42
/openconfig/interfaces/interface/state/counters/in-octets{name=ethernet-1/1} value=123456 updated=2026-09-08T10:00:00Z
```

Status fields:

| Field | Meaning |
|-------|---------|
| `stream` | `not_ready`, `connected`, or `reconnecting` |
| `synchronized` | Whether a `sync_response` was received on the active stream |
| `reconnect` | Reconnect attempts since the last successful stream |
| `samples` | Number of cached path values |
| `last_error` | Present when the stream failed (TLS, auth, network, and so on) |

### `agent gnmi preview-metrics`

Subscribes to the device and runs the same reporting path as the gNMI check,
printing metric names, values, and tags instead of sending them to the intake.

```bash
./bin/agent/agent gnmi preview-metrics --instance 0 --once -c bin/agent/dist
```

`--once` waits for stream synchronization (up to `--timeout`, default 30s),
collects metrics once, and exits. Without `--once`, metrics are printed every
`--interval` (default 2s).

#### Example output

```
target=127.0.0.1:57401 transport=tls encoding=json_ietf profile="interface-stats"
--- stream=connected synchronized=true reconnect=0 samples=42 metrics=18 metadata=1
gauge snmp.ifInOctets 123456 host=- tags=[device_id:default:127.0.0.1,device_ip:127.0.0.1,snmp_device:127.0.0.1,snmp_host:srl2,interface:ethernet-1/1,...]
rate snmp.ifInOctets.rate 1000 host=- tags=[...]
metadata event_type=network-devices-metadata
{
  "collect_timestamp": 123,
  "devices": [
    {
      "id": "default:127.0.0.1",
      "ip_address": "127.0.0.1",
      "name": "srl2"
    }
  ],
  "interfaces": [
    {
      "device_id": "default:127.0.0.1",
      "name": "ethernet-1/1"
    }
  ]
}
```

By default, preview includes:

- Profile metrics (`snmp.*` from the loaded profile)
- Derived metrics (interface bandwidth usage, memory usage)
- Operational metrics (`datadog.gnmi.*`)
- Interface status (`snmp.interface.status`) when inventory is complete
- Metadata payloads (`network-devices-metadata` events) when inventory is complete

Metadata is printed as pretty JSON after the metric lines. Use
`--include-metadata=false` to skip it.

### `agent check gnmi`

Runs the full check once through the collector, including sender submission. Use
this after the troubleshooting commands to validate end-to-end behavior.

```bash
./bin/agent/agent check gnmi --instance-filter '.address == "127.0.0.1" and .port == 57401' \
  -c bin/agent/dist
```

### `agent status`

When gNMI instances are configured, the Agent status output includes a gNMI
section with per-device stream state, sample counts, and last errors.

```bash
./bin/agent/agent status -c bin/agent/dist
```

## Shared flags

These flags are available on both `subscribe` and `preview-metrics`:

| Flag | Default | Description |
|------|---------|-------------|
| `--address` | | Device address |
| `--port` | `57400` | gNMI gRPC port |
| `--username` | | Authentication username |
| `--password` | | Authentication password |
| `--profile` | | Profile name or path under `conf.d/gnmi.d/profiles/` |
| `--encoding` | `json_ietf` | `proto`, `json`, or `json_ietf` |
| `--use-tls` | `false` | Enable TLS |
| `--insecure-skip-verify` | `false` | Skip TLS certificate verification |
| `--collect-topology` | `false` | Subscribe to LLDP topology paths |
| `--interval` | `2s` | Print interval |
| `--fast-reconnect` | `true` | Shorter reconnect backoff for interactive use |

`preview-metrics` adds:

| Flag | Default | Description |
|------|---------|-------------|
| `--once` | `false` | Collect once after sync, then exit |
| `--wait-sync` | `true` | Wait for stream sync before printing metrics |
| `--timeout` | `30s` | Max wait for sync when using `--once` |
| `--include-health` | `true` | Include `datadog.gnmi.*` metrics |
| `--include-interface-status` | `true` | Include `snmp.interface.status` when ready |
| `--include-metadata` | `true` | Include `network-devices-metadata` JSON payloads |

## Recommended workflow

1. **`subscribe --list-paths`** — confirm the profile expands to the expected YANG
   paths.
2. **`subscribe`** — confirm the stream connects, synchronizes, and values update.
3. **`preview-metrics --once`** — confirm metrics and tags match expectations.
4. **`check gnmi`** — run the full check path.
5. **`agent run`** — enable continuous collection.

## Common issues

### `read .../conf.d/gnmi.d/conf.yaml: no such file or directory`

`-c` points at the wrong directory, or `confd_path` in `datadog.yaml` does not
match where your `conf.yaml` lives. Use `-c bin/agent/dist` or pass
`--conf-file` explicitly.

### `waiting for stream synchronization` with `--once`

The stream has not received `sync_response` yet. Wait longer (`--timeout 60s`),
or verify the target is up with `subscribe`. If `last_error` is set, fix
transport or auth first.

### `no metrics emitted`

Samples may be stale relative to `min_collection_interval`, or interface
inventory may be incomplete. Compare raw values with `subscribe` first.

### Profile not found

Profiles resolve from `confd_path/gnmi.d/profiles/`. When using `--conf-file`,
the parent `conf.d` directory is inferred automatically for profile loading.

## Further reading

- Package layout and extension points:
  `pkg/collector/corechecks/network-devices/gnmi/AGENTS.md`
- Instance configuration template: `cmd/agent/dist/conf.d/gnmi.d/conf.yaml.default`
- Built-in help: `./bin/agent/agent gnmi --help`
