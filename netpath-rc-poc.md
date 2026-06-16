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

Build and run the Agent from this branch:

```bash
dda inv agent.build --build-exclude=systemd
./bin/agent/agent run -c bin/agent/dist/datadog.yaml
```

The Agent must run with Remote Config enabled and a staging API key that can receive RC configs. No Network Path-specific local flag is required.

Create a DEBUG config targeted to the Agent hostname:

```bash
export TARGET_HOSTNAME="$(hostname)"
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

Expected result:

- RC admin shows the DEBUG config as acknowledged by the Agent.
- Agent logs show the `network-path-remote-config` provider collecting a new config.
- Agent status shows a `network_path` check sourced from `remote_config_debug/network_path/...`.
- Emitted Network Path payloads are scheduled test runs and include the POC provenance tags.

To update the config, increment `attributes.version` by one and use the RC debug API `PATCH` endpoint for the returned config ID. To remove the test, either patch the contents to `{"poc":"network_path_scheduled_tests","type":"scheduled","configs":[]}` or delete the RC config.

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
