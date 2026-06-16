# Network Path Scheduled Tests via Remote Config DEBUG POC

## Goal

This POC proves that the Agent can receive Network Path Scheduled Test definitions through Remote Config and run them as ordinary `network_path` integration instances, without an Agent restart and without adding a dedicated scheduler.

The POC uses the staging-only Remote Config `DEBUG` product. Production follow-up work can move the same listener contract to a real Network Path RC product.

## Design

The Agent subscribes to the `DEBUG` RC product whenever Remote Config is enabled. Because `DEBUG` is shared by internal POCs, the Network Path listener only processes configs with this top-level marker:

```json
{
  "poc": "network_path_scheduled_tests"
}
```

All other `DEBUG` configs are ignored by this listener and do not receive an apply status from it.

Marked configs must use `type: "scheduled"`. Each RC config contributes one `network_path` integration config with one instance per entry in `configs`. Autodiscovery then reconciles the generated integration configs and the existing `network_path` check performs the scheduled traceroute runs.

YAML-configured `network_path` checks continue to run. The POC does not implement custom duplicate detection; any de-duplication is whatever the existing autodiscovery scheduler already provides.

## Payload

Canonical DEBUG payload:

```json
{
  "poc": "network_path_scheduled_tests",
  "type": "scheduled",
  "configs": [
    {
      "test_id": "np-test-001",
      "hostname": "api.example.com",
      "port": 443,
      "protocol": "TCP",
      "max_ttl": 30,
      "timeout_ms": 1000,
      "interval_sec": 60,
      "source_service": "frontend",
      "destination_service": "api",
      "tcp_method": "syn",
      "traceroute_queries": 3,
      "e2e_queries": 50,
      "tags": ["env:prod", "team:network"]
    }
  ]
}
```

Field mapping into the existing `network_path` integration schema:

| RC field | Integration field |
|---|---|
| `hostname` | `hostname` |
| `port` | `port` |
| `protocol` | `protocol` |
| `max_ttl` | `max_ttl` |
| `timeout_ms` | `timeout` |
| `interval_sec` | `min_collection_interval` |
| `source_service` | `source_service` |
| `destination_service` | `destination_service` |
| `tcp_method` | `tcp_method` |
| `traceroute_queries` | `traceroute_queries` |
| `e2e_queries` | `e2e_queries` |
| `tags` | `tags` |

`test_id` is optional and is added as `network_path.test_id:<test_id>`.

The listener also adds these provenance tags:

```text
network_path.config_source:remote_config
network_path.rc_product:debug
network_path.rc_config_id:<rc config id or path>
network_path.rc_config_version:<rc config version>
```

The `network_path.*` tag prefixes above are reserved for the listener. If users provide those tags directly, the config is rejected.

`configs: []` is valid and schedules no tests. This is useful to remove all tests contributed by a config while keeping the RC object.

## Validation And Apply Semantics

The listener validates marked configs before scheduling them:

- `type` must be `scheduled`
- `configs` must be present
- each test must include `hostname`
- numeric values must be in valid ranges
- `protocol` must be `TCP`, `UDP`, or `ICMP` when set
- `tcp_method` must be `syn`, `sack`, `prefer_sack`, or `syn_socket` when set
- reserved listener tags are rejected

If a marked config is invalid, the listener reports `ApplyStateError` for that RC config. If a previous valid version of the same RC config was already scheduled, it remains active. Deleting the RC config removes its scheduled tests.

## Manual Validation Against Staging RC

The staging validation exercised the full local path:

```text
RC DEBUG -> core Agent RC client -> network-path RC provider -> Autodiscovery scheduler -> network_path check -> system-probe traceroute
```

### Local Configuration

Use the same dev config directory for the Agent and system-probe:

```bash
export CONFIG_DIR=/Users/alexandre.yang/go/src/github.com/DataDog/datadog-agent/dev/dist
export AGENT_CONFIG="$CONFIG_DIR/datadog.yaml"
```

`datadog.yaml` must include:

- a staging RC-enabled API key
- staging site configuration, for example `site: datad0g.com`
- `remote_configuration.enabled: true`
- a hostname matching the RC DEBUG target

The successful staging run used hostname `alex-test02`.

`system-probe.yaml` must enable traceroute support. The session used:

```yaml
traceroute:
  enabled: true

network_config:
  enabled: true

log_level: debug
```

### Build

Build both binaries from this branch:

```bash
GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
  dda inv agent.build --build-exclude=systemd --skip-assets

GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
  dda inv system-probe.build
```

### Run System-Probe

Start system-probe first. It owns the traceroute endpoint used by the `network_path` check:

```bash
mkdir -p run run/ipc

sudo ./bin/system-probe/system-probe run \
  -c "$CONFIG_DIR" \
  --datadogcfgpath "$CONFIG_DIR"
```

`sudo` is required for the traceroute raw socket path on macOS. Without it, the Agent can receive and schedule the RC config, but the check fails with a permission error from system-probe.

### Run The Agent

In another terminal, run the Agent against the same config:

```bash
./bin/agent/agent run \
  -c "$AGENT_CONFIG"
```

No Network Path-specific local Agent flag is required. The listener is active whenever Remote Config is enabled; this matches the RFC decision for the POC.

### Create A DEBUG Config

Create a DEBUG config targeted to the Agent hostname:

```bash
export TARGET_HOSTNAME="alex-test02"
export CONFIG_CONTENT="$(jq -nc '{
  poc: "network_path_scheduled_tests",
  type: "scheduled",
  configs: [{
    test_id: "np-poc-001",
    hostname: "api.datadoghq.com",
    port: 443,
    protocol: "TCP",
    timeout_ms: 1000,
    interval_sec: 60,
    tags: ["env:poc"]
  }]
}')"

export BODY="$(jq -n \
  --arg contents "$CONFIG_CONTENT" \
  --arg hostname "$TARGET_HOSTNAME" \
  '{"data":{"type":"configs","id":"network-path-scheduled-poc","attributes":{"contents":$contents,"version":1,"target":{"hostname":$hostname}}}}')"

http --ignore-stdin \
  https://rc-debug-api.us1.staging.dog/internal/remote_config/products/debug/datadog/configs \
  "$(ddtool auth token rapid-remote-config --datacenter us1.staging.dog --http-header)" \
  --raw="$BODY"
```

The RC API returns the concrete config ID. In the staging session, it was `network-path-scheduled-poc-1781637194`.

### Update The DEBUG Config

To update an existing DEBUG config, read the current config first and send its current `attributes.version` in the `PATCH` request. The RC API treats that version as a precondition and increments it on success.

This example updates the POC config to monitor several well-known domains:

```bash
export CONFIG_ID="network-path-scheduled-poc-1781637194"
export BASE_URL="https://rc-debug-api.us1.staging.dog/internal/remote_config/products/debug/datadog/configs"
export RC_AUTH_HEADER="$(ddtool auth token rapid-remote-config --datacenter us1.staging.dog --http-header)"

export CURRENT="$(http --ignore-stdin GET "$BASE_URL/$CONFIG_ID" "$RC_AUTH_HEADER")"
export CURRENT_VERSION="$(printf '%s' "$CURRENT" | jq -r '.data.attributes.version')"
export TARGET_HOSTNAME="$(printf '%s' "$CURRENT" | jq -r '.data.attributes.target.hostname')"

export CONFIG_CONTENT="$(jq -nc '{
  poc: "network_path_scheduled_tests",
  type: "scheduled",
  configs: [
    {
      test_id: "np-poc-001",
      hostname: "api.datadoghq.com",
      port: 443,
      protocol: "TCP",
      timeout_ms: 1000,
      interval_sec: 60,
      tags: ["env:poc", "source:rc-debug"]
    },
    {
      test_id: "np-poc-google",
      hostname: "www.google.com",
      port: 443,
      protocol: "TCP",
      timeout_ms: 1000,
      interval_sec: 60,
      tags: ["env:poc", "source:rc-debug", "endpoint:google"]
    },
    {
      test_id: "np-poc-cloudflare",
      hostname: "www.cloudflare.com",
      port: 443,
      protocol: "TCP",
      timeout_ms: 1000,
      interval_sec: 60,
      tags: ["env:poc", "source:rc-debug", "endpoint:cloudflare"]
    },
    {
      test_id: "np-poc-github",
      hostname: "github.com",
      port: 443,
      protocol: "TCP",
      timeout_ms: 1000,
      interval_sec: 60,
      tags: ["env:poc", "source:rc-debug", "endpoint:github"]
    },
    {
      test_id: "np-poc-wikipedia",
      hostname: "www.wikipedia.org",
      port: 443,
      protocol: "TCP",
      timeout_ms: 1000,
      interval_sec: 60,
      tags: ["env:poc", "source:rc-debug", "endpoint:wikipedia"]
    }
  ]
}')"

export BODY="$(jq -n \
  --arg id "$CONFIG_ID" \
  --arg contents "$CONFIG_CONTENT" \
  --arg hostname "$TARGET_HOSTNAME" \
  --argjson version "$CURRENT_VERSION" \
  '{"data":{"type":"configs","id":$id,"attributes":{"contents":$contents,"version":$version,"target":{"hostname":$hostname}}}}')"

http --ignore-stdin PATCH "$BASE_URL/$CONFIG_ID" "$RC_AUTH_HEADER" --raw="$BODY"
```

Verify the applied version and endpoints:

```bash
http --ignore-stdin GET "$BASE_URL/$CONFIG_ID" "$RC_AUTH_HEADER" |
  jq '{
    id: .data.id,
    version: .data.attributes.version,
    target: .data.attributes.target,
    configs: (
      .data.attributes.contents |
      fromjson |
      .configs |
      map({test_id, hostname, port, protocol, interval_sec})
    )
  }'
```

The staging config was updated to version `3` with the five endpoints above.

### Expected Result

- RC admin shows the DEBUG config as acknowledged by the Agent.
- Agent logs show the `network-path-remote-config` provider collecting a new config.
- Agent status shows a `network_path` check sourced from `remote_config_debug/network_path/...`.
- Emitted Network Path payloads are scheduled test runs and include the POC provenance tags.

The successful local Agent status included:

```text
Instance ID: network_path:ee77807503a2bae1 [OK]
Configuration Source: remote_config_debug/network_path/network-path-scheduled-poc-1781637194/ef1454580708448bf6cdd91f73ff903ab58618a67240d81c846ab6802ee8cd42[0]
Total Runs: 1
Metric Samples: Last Run: 2, Total: 2
Network Path: Last Run: 1, Total: 1
Last Successful Execution Date: 2026-06-16 21:28:35 CEST
```

This validates the RC DEBUG to scheduled Network Path execution path locally. It does not by itself prove product UI visibility in staging; that requires checking the backend/UI after payload ingestion.

### Troubleshooting Notes

- `dial unix .../run/sysprobe.sock: connect: no such file or directory`: system-probe is not running or did not create the socket.
- `listen unix .../run/sysprobe.sock: bind: no such file or directory`: create the local runtime directory with `mkdir -p run run/ipc` before starting system-probe.
- `Permission denied` or `failed to create raw socket: operation not permitted`: restart system-probe with `sudo`.
- Initial system-probe `Registering with Core Agent ... connection refused` messages are expected if system-probe starts before the Agent. It registers once the Agent gRPC server is available.
- To remove all tests from a DEBUG config without deleting the RC object, patch contents to `{"poc":"network_path_scheduled_tests","type":"scheduled","configs":[]}`.

## POC Scope

Included:

- DEBUG product listener
- scheduled Network Path tests
- unit tests for parsing, validation, translation, cache behavior, and apply status
- manual staging RC validation steps

Out of scope:

- Dynamic Tests
- production `NETWORK_PATH` RC product registration and schema
- UI or RC API changes
- custom scheduler or custom duplicate detection
- secret resolution
- automated staging RC E2E tests
