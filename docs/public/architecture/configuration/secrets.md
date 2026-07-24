# Secrets management

-----

Secrets management lets users keep credentials out of plaintext configuration: any YAML string value of the form `ENC[handle]` is a *secret handle* that the Agent resolves at load time by exec-ing a backend program and substituting the returned value in memory. The subsystem is the `comp/core/secrets` component, wired into the [configuration load sequence](overview.md) so that `datadog.yaml`, sub-agent config files, and integration (check) configs are all resolved transparently. Resolved values live only in memory, in a dedicated `secret` config layer, and are registered with the scrubber so they never appear in logs, flares, or metadata.

User-facing documentation is at [docs.datadoghq.com/agent/configuration/secrets-management](https://docs.datadoghq.com/agent/configuration/secrets-management); this page covers the internals.

## Key packages

| Path | Purpose |
| --- | --- |
| [`comp/core/secrets/def/component.go`](<<<SRC>>>/comp/core/secrets/def/component.go) | Component interface: `Configure`, `Resolve`, `SubscribeToChanges`, `Refresh`, `RefreshNow`; `ConfigParams` |
| [`comp/core/secrets/impl/secrets.go`](<<<SRC>>>/comp/core/secrets/impl/secrets.go) | The resolver: handle cache, backend-mode selection, refresh loop, audit file, status/flare providers |
| [`comp/core/secrets/impl/fetch_secret.go`](<<<SRC>>>/comp/core/secrets/impl/fetch_secret.go) | Backend exec: JSON payload construction, output-size limiting, multi-backend grouping |
| [`comp/core/secrets/utils/utils.go`](<<<SRC>>>/comp/core/secrets/utils/utils.go) | `IsEnc` and the YAML walker that records the path to every handle |
| [`pkg/util/filesystem/rights_windows.go`](<<<SRC>>>/pkg/util/filesystem/rights_windows.go) | Windows ACL permission checks for the backend executable (`CheckRights`) |
| [`comp/core/secrets/impl/rotating_ndrecords.go`](<<<SRC>>>/comp/core/secrets/impl/rotating_ndrecords.go) | ND-JSON audit file with size-based rotation |
| [`comp/core/secrets/noop-impl`](<<<SRC>>>/comp/core/secrets/noop-impl) | No-op implementation (used by system-probe's run command) |
| [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go) | `resolveSecrets`: configures the resolver from `datadog.yaml` and pushes resolved values back into the config |
| [`cmd/secret-generic-connector`](<<<SRC>>>/cmd/secret-generic-connector) | The embedded generic connector binary (native backends: AWS, HashiCorp Vault, Kubernetes secrets, files, ...) |
| [`cmd/agent/subcommands/secret/command.go`](<<<SRC>>>/cmd/agent/subcommands/secret/command.go) | `agent secret` / `agent secret refresh` CLI |
| [`pkg/util/scrubber`](<<<SRC>>>/pkg/util/scrubber) | Scrubbing engine that resolved values are registered with |

## Resolution flow

At startup, `resolveSecrets` in [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go) runs as part of `LoadDatadog`, after the file and env layers are loaded and after proxy resolution (so proxy settings may themselves be `ENC[...]` handles):

```text
LoadDatadog
  └── resolveSecrets(config, resolver, origin="datadog.yaml")
        ├── resolver.Configure(ConfigParams{...})        ← ~15 secret_* settings
        ├── yaml.Marshal(config.AllSettings())            ← full config as YAML
        ├── resolver.SubscribeToChanges(callback)
        └── resolver.Resolve(yaml, origin, ..., notify=true)
              ├── walker finds every ENC[...] string + its path
              ├── fetchSecret(handles) → exec backend once per backend
              └── for each resolved handle:
                    callback(handle, origin, path, old, new)
                      └── configAssignAtPath(config, path, value)
                            └── config.Set(path, value, SourceSecret)
```

The walker in [`comp/core/secrets/utils/utils.go`](<<<SRC>>>/comp/core/secrets/utils/utils.go) descends maps and slices and records the path to each handle; only string values that are *entirely* an `ENC[...]` token are treated as handles (`IsEnc`). Resolved values enter the config through the `SubscribeToChanges` callback at `SourceSecret` (priority 7 — above env vars and fleet policies, below configsync-mirrored and runtime values; see [the source table](overview.md#the-layered-node-tree-model)). `configAssignAtPath` handles paths that are not schema keys, such as `additional_endpoints."https://url.com".0`, by walking up to the nearest known key and mutating the compound value in place.

The same `Resolve` API is used by [Autodiscovery](../checks/autodiscovery.md) for integration configs (with per-config `origin`, image name, and Kubernetes namespace, enabling the scoping rules below), by the security-agent and cluster-agent for their config files, and manually by the otel-agent ([`cmd/otel-agent/config/agent_config.go`](<<<SRC>>>/cmd/otel-agent/config/agent_config.go)).

## Backend modes

`Configure` in [`comp/core/secrets/impl/secrets.go`](<<<SRC>>>/comp/core/secrets/impl/secrets.go) selects exactly one mode; if several are set, the higher one wins and a warning names the ignored settings.

1. **`secret_backend_command`** — a user-supplied executable. The Agent enforces strict file permissions before every exec (`filesystem.CheckRights`): on Linux, owner-only execute unless `secret_backend_command_allow_group_exec_perm` is set; on Windows, an ACL check restricted to Administrators, LocalSystem, and `ddagentuser` ([`pkg/util/filesystem/rights_windows.go`](<<<SRC>>>/pkg/util/filesystem/rights_windows.go)).
1. **`secret_backend_type` + `secret_backend_config`** — runs the *embedded* [`secret-generic-connector`](<<<SRC>>>/cmd/secret-generic-connector) binary that ships with the Agent, from the embedded bin directory (the install path for the cluster-agent flavor). The JSON payload gains `"type"` and `"config"` keys, and the permission check is relaxed (`embeddedBackendPermissiveRights`) since the binary is Agent-owned.
1. **`multi_secret_backends`** — a map of named backends for the embedded connector. Handles take the form `ENC[backendID;secretKey]`; handles are grouped per backend and fetched with one exec per backend ([`fetch_secret.go`](<<<SRC>>>/comp/core/secrets/impl/fetch_secret.go)). Unprefixed `ENC[...]` handles are rejected unless `secret_backend_type` is also set as the fallback backend.

The exec protocol is versioned JSON over stdin/stdout:

```text
stdin:  {"version": "1.1", "secrets": ["handle1", "handle2"],
         "secret_backend_timeout": 30, "type": "...", "config": {...}}
stdout: {"handle1": {"value": "s3cr3t"},
         "handle2": {"error": "access denied"}}
```

Output is capped at `secret_backend_output_max_size` (1 MiB) via a limiting buffer; the exec is killed after `secret_backend_timeout` seconds (30 by default). Per-handle errors, missing handles, and empty resolved values each fail that handle individually with a descriptive error; command failure or invalid JSON fails the whole batch. stderr is always logged so backend scripts can emit troubleshooting output into the Agent log. `secret_backend_remove_trailing_line_break` strips trailing newlines from values — a common footgun with `Get-Secret`-style scripts.

## Refresh and audit

Resolution is not necessarily one-shot. Three triggers re-fetch every known handle:

1. **Periodic**: `secret_refresh_interval` (seconds, 0 = disabled). `secret_refresh_scatter` (default true) randomizes the *first* tick within the interval so a fleet of agents does not stampede the secrets backend simultaneously; subsequent ticks use the plain interval.
1. **On demand**: `agent secret refresh` hits the `/agent/secret/refresh` endpoint on the command API ([`cmd/agent/subcommands/secret/command.go`](<<<SRC>>>/cmd/agent/subcommands/secret/command.go)); trace-agent, process-agent, and security-agent expose equivalent `/secret/refresh` routes.
1. **On API-key rejection**: a throttled refresh (`secret_refresh_on_api_key_failure_interval`, minutes) fires when intake responds with an invalid-API-key error, so a rotated key is picked up without waiting for the next tick.

On refresh, changed values are pushed to every `SubscribeToChanges` subscriber, which re-`Set`s them at `SourceSecret`; the [forwarder](../pipelines/forwarder.md) and other components observe the change through config `OnUpdate` notifications. Each refresh appends to an audit file `${run_path}/secret-audit-file.json` (ND-JSON, rotated at `secret_audit_file_max_size`, default 1 MiB) recording which handles changed and when — the file is included in [flares](../operations/flare.md).

`agent secret` (without arguments) prints resolver diagnostics: the active backend mode, executable permissions, every handle, and the config paths where each is used.

## Kubernetes scoping for integration configs

When integration configs come from Autodiscovery, three settings restrict which secrets a pod's config may reference (all resolved per-container, using the image name and namespace passed to `Resolve`):

| Key | Effect |
| --- | --- |
| `secret_scope_integration_to_their_k8s_namespace` | A `k8s_secret@ns/name/key` handle must match the pod's own namespace |
| `secret_allowed_k8s_namespace` | Allowlist of namespaces whose secrets may be referenced |
| `secret_image_to_handle` | Map of container image → allowed handle list |

## Scrubbing and leak prevention

Every resolved value is registered as a scrub word with [`pkg/util/scrubber`](<<<SRC>>>/pkg/util/scrubber), so it is masked in logs, status output, and flares even if it appears embedded in another string. Two config-API guarantees back this up:

1. The `secret` layer is excluded from `SourceProvided`, the pseudo-source used by metadata payloads (`inventoryagent` and friends) — resolved secrets never ship in "provided configuration" telemetry.
1. `AllSettingsWithoutSecrets` / `AllSettingsWithoutDefaultOrSecrets` drop the secret layer entirely; the config component's flare provider uses them for `runtime_config_dump.yaml`. If you add a new "dump all settings" API, use these or you will leak credentials.

`IsValueFromSecret(value)` lets other components ask whether a string ever came from a secret handle (used, for example, to decide whether an API key change came from rotation).

## Configuration reference

| Key | Default | Meaning |
| --- | --- | --- |
| `secret_backend_command` | "" | User executable resolving handles |
| `secret_backend_arguments` | [] | Extra CLI arguments for the command |
| `secret_backend_timeout` | 30 | Exec timeout (seconds) |
| `secret_backend_output_max_size` | 1 MiB | stdout/stderr cap |
| `secret_backend_command_allow_group_exec_perm` | false | Relax the owner-only permission check (Linux) |
| `secret_backend_remove_trailing_line_break` | false | Strip trailing newlines from resolved values |
| `secret_backend_type` / `secret_backend_config` | "" / — | Native backend via the embedded connector |
| `multi_secret_backends` | — | Named backends; handles become `ENC[backendID;secretKey]` |
| `secret_refresh_interval` | 0 | Periodic refresh (seconds) |
| `secret_refresh_scatter` | true | Randomize first refresh tick |
| `secret_refresh_on_api_key_failure_interval` | 0 | Throttled refresh on API-key rejection (minutes) |
| `secret_audit_file_max_size` | 1 MiB | Audit file rotation threshold |
| `secret_scope_integration_to_their_k8s_namespace` / `secret_allowed_k8s_namespace` / `secret_image_to_handle` | — | Kubernetes scoping (above) |

system-probe.yaml declares its own `secret_backend_*` keys in its schema, but see the gotcha below.

## Deployment-mode differences

1. **Windows**: permission checks use ACLs instead of Unix modes; the embedded connector is `secret-generic-connector.exe` under the install's embedded bin directory.
1. **Cluster Agent**: the embedded connector is looked up in the install path rather than the embedded bin path (flavor check in `Configure`).
1. **Containers**: the backend command must exist inside the Agent container image and satisfy the permission checks as seen by the container's `dd-agent` user; the official images ship the embedded connector, making `secret_backend_type` the low-friction option.

## Gotchas

1. **system-probe does not resolve secrets in-process.** Its run command wires the noop secrets module ([`cmd/system-probe/subcommands/run/command.go`](<<<SRC>>>/cmd/system-probe/subcommands/run/command.go)), so `ENC[...]` handles in `system-probe.yaml` are not resolved there even though the `secret_backend_*` keys exist in its schema. Shared values resolved by the core agent can reach it via configsync/configstream (see [the overview](overview.md#config-propagation-between-processes)).
1. **Only whole-string handles resolve.** `api_key: ENC[foo]` works; `dd_url: https://ENC[foo].example.com` does not — the walker matches values that are exactly one `ENC[...]` token.
1. **Empty resolved values are errors.** A backend returning `""` for a handle fails that handle; combined with `secret_backend_remove_trailing_line_break`, a value that was only a newline becomes an error rather than an empty setting.
1. **`secret_backend_command` wins silently-ish.** If both a command and a native backend type are configured, the command is used and the others are ignored with a single startup warning.
1. **Refresh only re-resolves known handles.** Handles are discovered when a config (by `origin`) is first resolved; a handle added to `datadog.yaml` after startup is not picked up by refresh — the file layer itself is never re-read at runtime.
1. **Resolution operates on the merged view.** `resolveSecrets` marshals `AllSettings()` — the already-merged config — so a handle can come from any layer (file, env var, fleet policy), and only the layer that currently wins for a key is inspected. The resolved plaintext then lands at `SourceSecret` (priority 7), above all of those layers.
