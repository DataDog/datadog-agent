# The configuration system

-----

The Agent configuration system is an in-house, layered key/value store that replaced Viper. Every process in the Agent family (core agent, trace-agent, process-agent, security-agent, system-probe, cluster-agent, otel-agent, serverless) reads its settings through the same machinery: a set of *source layers* — defaults, file, environment variables, fleet policies, secrets, remote config, CLI, and a few internal ones — each stored as a tree of nodes and merged by per-leaf source priority into a single root tree that all `Get*` calls read from. Every setting must be declared up front; the set of declarations *is* the schema, and it is sealed once at startup.

This page covers the model, the load sequence, environment-variable binding, sub-agent config files, and how resolved configuration propagates between processes. Secrets resolution is covered in [Secrets management](secrets.md), runtime mutation in [Runtime settings](runtime-settings.md), and the remote-config source in [Remote configuration](remote-config.md).

## Key packages

| Path | Purpose |
| --- | --- |
| [`pkg/config/model/types.go`](<<<SRC>>>/pkg/config/model/types.go) | Core interfaces (`Reader`, `Writer`, `Config`, `BuildableConfig`), the `Source` enum and its priority table |
| [`pkg/config/nodetreemodel`](<<<SRC>>>/pkg/config/nodetreemodel) | `ntmConfig`: the layered tree implementation — one tree per source, merged into `root` |
| [`pkg/config/create/new.go`](<<<SRC>>>/pkg/config/create/new.go) | `NewConfig(name)` constructor; swaps in a schema builder when the binary runs as `createschema` |
| [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go) | The global `datadog`/`systemProbe` singletons, `InitConfigObjects`, `LoadDatadog`, proxy/FIPS/unknown-key logic |
| [`pkg/config/setup/common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go) | The bulk of `BindEnvAndSetDefault` declarations for `datadog.yaml` |
| [`pkg/config/setup/system_probe.go`](<<<SRC>>>/pkg/config/setup/system_probe.go) | All `system-probe.yaml` setting declarations |
| [`pkg/config/setup/fixup_init.go`](<<<SRC>>>/pkg/config/setup/fixup_init.go) | Post-declaration fixups and registration of override funcs |
| [`pkg/config/structure/unmarshal.go`](<<<SRC>>>/pkg/config/structure/unmarshal.go) | `structure.UnmarshalKey` — decode a config subtree into a struct |
| [`pkg/config/basic/convert.go`](<<<SRC>>>/pkg/config/basic/convert.go) | `ConvertToDefaultType` — casts incoming values to the type of the declared default |
| [`pkg/config/env`](<<<SRC>>>/pkg/config/env) | Environment and feature detection (`DetectFeatures`, `IsKubernetes`, `IsECSFargate`, socket probing) |
| [`pkg/config/schema/schema.go`](<<<SRC>>>/pkg/config/schema/schema.go) | Embedded compressed JSON schemas + `ValidateCoreConfig`/`ValidateSystemProbeConfig` |
| [`pkg/config/buildschema`](<<<SRC>>>/pkg/config/buildschema) | The `createschema` implementation that records declarations into a JSON-schema document |
| [`pkg/config/config_template.yaml`](<<<SRC>>>/pkg/config/config_template.yaml) | Documented template used to generate `datadog.yaml.example` and enrich the schema with docs |
| [`pkg/config/mock/mock.go`](<<<SRC>>>/pkg/config/mock/mock.go) | Test mock: a fresh, isolated config instead of the global singletons |
| [`comp/core/config`](<<<SRC>>>/comp/core/config) | Fx component wrapping the global config: per-binary `Params`, file/fleet/CLI load, flare provider |
| [`comp/core/configsync`](<<<SRC>>>/comp/core/configsync) | Sub-process poller mirroring core-agent values via `/config/v1` |
| [`comp/core/configstream`](<<<SRC>>>/comp/core/configstream) | Core-agent gRPC producer streaming config snapshots and updates to remote agents |
| [`pkg/system-probe/config/config.go`](<<<SRC>>>/pkg/system-probe/config/config.go) | system-probe config bootstrap: loads `system-probe.yaml` and computes enabled modules |
| [`tasks/schema`](<<<SRC>>>/tasks/schema) | Invoke tasks: `schema.generate`, `schema.codegen`, `schema.template`, `schema.locate` |

## The layered node-tree model

The runtime implementation is `ntmConfig` in [`pkg/config/nodetreemodel/config.go`](<<<SRC>>>/pkg/config/nodetreemodel/config.go). It holds one node tree per source (`defaults`, `file`, `envs`, `fleetPolicies`, `configPostInit`, `secrets`, `localConfigProcess`, `runtime`, `remoteConfig`, `cli`, ...) plus a merged `root` tree. Merging is per-leaf: when two layers define the same leaf, the leaf whose `Source` has the higher priority wins (`nodeImpl.Merge` in [`node.go`](<<<SRC>>>/pkg/config/nodetreemodel/node.go)); a `nil`-valued leaf never overrides an existing subtree.

The `Source` enum and its priorities live in [`pkg/config/model/types.go`](<<<SRC>>>/pkg/config/model/types.go). Higher priority wins:

| Priority | Source | Written by |
| --- | --- | --- |
| -1 | `schema` | Declared keys with no default |
| 0 | `default` | `BindEnvAndSetDefault` / `SetDefault` declarations |
| 1 | `unknown` | Tests only (`SetInTest` on undeclared keys) |
| 2 | `infra-mode` | `infrastructure_mode` overrides (basic, end_user_device, ...) |
| 3 | `file` | `datadog.yaml` and extra config files |
| 4 | `environment-variable` | `DD_*` env vars, snapshotted once at startup |
| 5 | `fleet-policies` | Policy files dropped by [Fleet Automation](../deployment/fleet.md) in `fleet_policies_dir` |
| 6 | `config-post-init` | Load-time computed values (proxy resolution, resolved auth-token and IPC-cert paths) |
| 7 | `secret` | Resolved `ENC[...]` handles (see [Secrets management](secrets.md)) |
| 8 | `local-config-process` | Values mirrored from the core agent by configsync |
| 9 | `agent-runtime` | Values the Agent computes about itself at runtime |
| 10 | `remote-config` | [Remote configuration](remote-config.md) overrides |
| 11 | `cli` | `agent config set`, per-command CLI flag overrides |

Two consequences of this table are easy to miss: fleet policies outrank environment variables, so a Fleet Automation config experiment overrides `DD_*` container env config; and CLI outranks remote config, which is why a remote-config `log_level` change is refused after someone runs `agent config set log_level` (see [Runtime settings](runtime-settings.md)).

Writes go through `Set(key, value, source)`: the value lands in that source's tree, is converted to the Go type of the declared default (`basic.ConvertToDefaultType`), gets re-merged into `root`, bumps a monotonically increasing `sequenceID`, and triggers every `OnUpdate` callback — outside the config lock, and skipped when the new value equals the current effective value. `UnsetForSource(key, source)` removes a value from one layer and splices in the value from the next-lower-priority layer; remote config uses it to withdraw overrides cleanly.

Introspection APIs are first-class: `GetSource(key)`, `GetAllSources(key)` (the value in every layer), `AllSettingsBySource()`, `IsConfigured(key)` (set by any non-default source), and `AllFlattenedSettingsWithSequenceID()` (used by configstream snapshots). `SourceProvided` is a pseudo-source meaning "everything above defaults" — it deliberately **excludes the secrets layer** so resolved secret values never leak into metadata payloads.

### Declaration and schema sealing

All settings are declared up front, mostly in [`pkg/config/setup/common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go), with `BindEnvAndSetDefault(key, default, optionalEnvVars...)`. After the declaration functions run, `BuildSchema()` seals the config: it resolves `${conf_path}`-style relative defaults against the per-OS paths in [`pkg/util/defaultpaths`](<<<SRC>>>/pkg/util/defaultpaths), materializes the env-var layer by reading `os.LookupEnv` for every bound variable, and merges all layers into `root`. After sealing, `SetDefault`, `BindEnvAndSetDefault`, and `SetEnvKeyReplacer` panic. Tests can opt into lazy rebuilds with `SetTestOnlyDynamicSchema(true)`; production never rebuilds.

The declarations double as a machine-readable schema. Running any agent binary as `agent createschema` makes [`create.NewConfig`](<<<SRC>>>/pkg/config/create/new.go) return a [`buildschema`](<<<SRC>>>/pkg/config/buildschema) builder instead of a live config, so every declaration is recorded into a JSON-schema document. The `dda inv schema.generate` task drives this, then enriches the output with descriptions parsed from [`config_template.yaml`](<<<SRC>>>/pkg/config/config_template.yaml), and splits it into the YAML files under [`pkg/config/schema/yaml`](<<<SRC>>>/pkg/config/schema/yaml). Those files are zstd-compressed and embedded back into the binary ([`pkg/config/schema/schema.go`](<<<SRC>>>/pkg/config/schema/schema.go)) for runtime validation and the `agent config print-agent-schema` command. The full schema toolchain — keywords, CLI, codegen back to Go — is documented at [datadoghq.dev/datadog-agent/agent-schema](https://datadoghq.dev/datadog-agent/agent-schema/), and `dda inv schema.locate <key>` finds any setting's schema node. To add a new setting, follow [the config how-to](https://datadoghq.dev/datadog-agent/how-to/go/config/).

## `DD_` environment variable binding

Binding is explicit: only declared keys read the environment. The rules, implemented in `buildEnvVars` ([`pkg/config/nodetreemodel/config.go`](<<<SRC>>>/pkg/config/nodetreemodel/config.go)):

1. The default env var name is `DD_` + the key upper-cased with `.` replaced by `_`: `logs_config.batch_wait` binds `DD_LOGS_CONFIG_BATCH_WAIT`.
1. Extra aliases can be declared per key (for example `dd_url` also binds `DD_DD_URL` and `DD_URL`); the first alias set to a non-empty value wins, in declaration order.
1. Empty env values are ignored — you cannot set a setting to the empty string via env var.
1. The env layer is snapshotted once at `BuildSchema()`. Later changes to the process environment are invisible (unlike Viper's live lookup).
1. Type conversion: a registered transformer wins — `ParseEnvSplitComma`, `ParseEnvSplitSpace`, `ParseEnvJSON` (used by `prometheus_http_sd.configs`, `dogstatsd_mapper_profiles`), plus legacy parsers in [`pkg/config/helper/env.go`](<<<SRC>>>/pkg/config/helper/env.go) such as `ParseEnvTraceSpan` for `apm_config.analyzed_spans`. Otherwise the string is cast to the type of the declared default via [`basic.ConvertToDefaultType`](<<<SRC>>>/pkg/config/basic/convert.go); notably, a `[]string` default means whitespace-split (`DD_TAGS="a:1 b:2"`).
1. Any `DD_*` env var not bound by either the datadog or the system-probe schema produces a startup warning (`findUnknownEnvVars` in [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go)), minus a hard-coded allowlist of serverless and tracer variables.

## Load sequence at startup

Configuration is assembled in two phases: a process-wide declaration phase that runs from Go `init()`, and a per-binary load phase driven by the [`comp/core/config`](<<<SRC>>>/comp/core/config) component.

```text
init() ── InitConfigObjects (pkg/config/setup/config.go)
  ├── create `datadog` and `systemProbe` configs (pkg/config/create)
  ├── initConfig(): run every declaration function
  │     (common components, core-agent, full-agent-only, InitSystemProbeConfig)
  ├── fixupInitConfig(): computed defaults + register override funcs
  └── BuildSchema() on both configs   ← env layer snapshotted, schema sealed

Fx startup ── comp/core/config newConfig → setupConfig (comp/core/config/setup.go)
  ├── resolve config file (-c flag, then default conf path)
  ├── LoadDatadog (pkg/config/setup/config.go)
  │     ├── loadCustom: ReadInConfig (file layer) + unknown-key /
  │     │   unknown-env-var / unexpected-unicode warning passes
  │     ├── LoadProxyFromEnv → proxy.* at SourceConfigPostInit
  │     ├── resolveSecrets → ENC[...] handles → SourceSecret
  │     ├── configureDelegatedAuth (cloud-fetched API keys)
  │     ├── conflicting-option checks, API-key sanitization,
  │     │   scrubber extra keys, setupFipsEndpoints
  │     └── defer: DetectFeatures, then ApplyOverrideFuncs
  ├── MergeFleetPolicy(fleet_policies_dir/datadog.yaml) → SourceFleetPolicies
  └── CLI overrides (WithCLIOverride) → SourceCLI
```

Details worth knowing about the load phase:

1. Per binary, [`comp/core/config/params.go`](<<<SRC>>>/comp/core/config/params.go) selects behavior: `NewAgentParams`, `NewSecurityAgentParams` (loads `datadog.yaml` then merges `security-agent.yaml`), `NewClusterAgentParams` (config name `datadog-cluster`), plus options `WithConfigName`, `WithExtraConfFiles` (`--extracfgpath`), `WithFleetPoliciesDirPath`, `WithCLIOverride`, and `WithIgnoreErrors`.
1. `LoadProxyFromEnv` merges `DD_PROXY_*` then the standard `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` (case-insensitive) into `proxy.*` at `SourceConfigPostInit`, and adds cloud-metadata IPs to `no_proxy` unless `use_proxy_for_cloud_metadata` is set. It runs before secrets resolution, so proxy values may themselves be `ENC[...]` handles.
1. Feature detection (`pkgconfigenv.DetectFeatures`) runs in a `defer` after file and env are loaded, gated by `autoconfig_from_environment`. It probes well-known sockets and env vars with a 500 ms timeout to detect Docker, containerd, CRI-O, Kubernetes, ECS/EKS Fargate, Podman, and more ([`pkg/config/env/environment_containers.go`](<<<SRC>>>/pkg/config/env/environment_containers.go)). The result drives [Autodiscovery](../checks/autodiscovery.md) listener/provider selection and hundreds of `env.IsFeaturePresent` checks. Reading a feature before detection has run panics.
1. `ApplyOverrideFuncs` ([`pkg/config/model/config_overrides.go`](<<<SRC>>>/pkg/config/model/config_overrides.go)) then applies registered mutation hooks — infra-mode overrides, the Windows fleet-config registry override, default-payload toggles.
1. Fleet policies come from `fleet_policies_dir` (config, `--fleetcfgpath`, or on Windows the registry via [`pkg/config/setup/config_windows.go`](<<<SRC>>>/pkg/config/setup/config_windows.go)). They merge at `SourceFleetPolicies` once, at startup only — no update notifications.

## Sub-agent configuration files

| Process | File(s) | Schema |
| --- | --- | --- |
| core agent | `datadog.yaml` (+ `--extracfgpath` files) | main schema |
| system-probe | `system-probe.yaml` | its own schema, second global singleton |
| security-agent | `datadog.yaml`, then `security-agent.yaml` merged over it | main schema |
| cluster-agent | `datadog-cluster.yaml` | main schema (different config name) |
| trace-agent / process-agent | `datadog.yaml` (namespaced `apm_config.*`, `process_config.*` sections) | main schema |
| otel-agent | `datadog.yaml` + OTel pipeline config | main schema plus keys added via `RevertFinishedBackToBuilder` |

**system-probe** is the only process with a genuinely separate schema: a second global config (`setup.SystemProbe()`), declared by `InitSystemProbeConfig` in [`pkg/config/setup/system_probe.go`](<<<SRC>>>/pkg/config/setup/system_probe.go). [`pkg/system-probe/config/config.go`](<<<SRC>>>/pkg/system-probe/config/config.go) loads `system-probe.yaml` (same directory as `datadog.yaml` by default), applies fleet policy, then `Adjust()` ([`adjust.go`](<<<SRC>>>/pkg/system-probe/config/adjust.go) and siblings) normalizes cross-dependent settings and computes the set of enabled modules (network_tracer, event_monitor, gpu, ...) from feature flags across *both* configs. `system_probe_config.enabled` is recomputed as "any module enabled" at `SourceAgentRuntime`. See [system-probe](../ebpf/system-probe.md).

**security-agent** has no separate schema; its settings (`runtime_security_config.*`, `compliance_config.*`) are declared in the main datadog schema, and `security-agent.yaml` is merged over `datadog.yaml` at the file layer.

**serverless** builds the component without Fx (`NewServerlessConfig`), and the `serverless` build tag swaps in [`config_init_serverless.go`](<<<SRC>>>/pkg/config/setup/config_init_serverless.go) so only the common declaration set runs, keeping the binary small. **otel-agent** builds its config before the Fx graph and calls the exported `pkgconfigsetup.ResolveSecrets` manually ([`cmd/otel-agent/config/agent_config.go`](<<<SRC>>>/cmd/otel-agent/config/agent_config.go)); it is also the only user of `RevertFinishedBackToBuilder()`, which re-opens a sealed config to add OTel keys.

## Unknown keys and validation

1. An unknown key in a YAML file logs `unknown key from YAML: <path>` at load ([`read_config_file.go`](<<<SRC>>>/pkg/config/nodetreemodel/read_config_file.go)) plus a startup `Unknown key in config file` warning; the value is still stored and readable, and reading it later does not warn again (the key is already recorded as unknown at load).
1. `Set()` on an unknown key is a no-op with an error log — a frequent source of "why didn't my setting apply" confusion.
1. A known setting present in YAML with no value (`key:`) keeps its default, but marks the section as existing for `HasSection` (relied on by OTLP ingest).
1. Keys are case-insensitive (lowercased everywhere), and dotted keys inside YAML (`"a.b.c": 1`) are expanded into nested maps.
1. `yaml.UnmarshalStrict` is tried first; duplicate keys in `datadog.yaml` log an error and fall back to lenient parsing.
1. JSON-schema validation of user YAML (`schema.ValidateCoreConfig`) exists and backs `agent config print-agent-schema`-adjacent tooling, but is not yet a hard startup gate.
1. Parse errors are wrapped in `ErrConfigFileNotFound` for Viper compatibility, and `comp/core/config` deliberately tolerates that error when no explicit `-c` path was given — an unparseable `datadog.yaml` in the default location does *not* stop most binaries; they run env-only.

## Config propagation between processes

The core agent is the source of truth for values it rewrites at runtime (refreshed secrets, delegated-auth API keys, remote config). Three mechanisms mirror or expose configuration to the other processes; transport-level details (TLS, auth tokens, sockets) are covered in [Inter-process communication](../processes/ipc.md).

1. **configsync (pull)** — [`comp/core/configsync`](<<<SRC>>>/comp/core/configsync): sub-processes (trace-agent, process-agent, security-agent, otel-agent, ...) poll `https://agent_ipc.host:agent_ipc.port/config/v1/` every `agent_ipc.config_refresh_interval` seconds (0 disables it). The server side is a dedicated mTLS listener ([`comp/api/api/apiimpl/server_ipc.go`](<<<SRC>>>/comp/api/api/apiimpl/server_ipc.go)) that serves only the allowlist `api.AuthorizedConfigPathsCore` ([`comp/api/api/def/component.go`](<<<SRC>>>/comp/api/api/def/component.go)): `api_key`, `app_key`, `site`, `dd_url`, `additional_endpoints`, `proxy.*`, and similar. Fetched values are written locally at `SourceLocalConfigProcess` — above secrets, below agent-runtime.
1. **configstream (push, gRPC)** — [`comp/core/configstream`](<<<SRC>>>/comp/core/configstream) (producer in the core agent, served only while the remote-agent registry is enabled — `remote_agent.registry.enabled`, default true) and [`comp/core/configstreamconsumer`](<<<SRC>>>/comp/core/configstreamconsumer) (opt-in via `remote_agent.configstream.consumer.enabled`). The `AgentSecure.StreamConfigEvents` RPC sends a full snapshot (every flattened setting with its original source) followed by incremental updates keyed by the config `sequenceID`; discontinuities trigger a resync. Settings are applied on the consumer preserving their original source, so precedence semantics survive across processes. Large snapshots are why `agent_ipc.grpc_max_message_size` defaults to 128 MiB.
1. **fetcher (diagnostics pull)** — [`pkg/config/fetcher/from_processes.go`](<<<SRC>>>/pkg/config/fetcher/from_processes.go): `agent flare` and `agent config`-style helpers that GET the full scrubbed runtime config from each process's settings API (ports in [Runtime settings](runtime-settings.md)).

## Configuration reference

Settings that govern the config system itself (all in `datadog.yaml` unless noted):

| Key | Default | Meaning |
| --- | --- | --- |
| `cmd_host` / `cmd_port` | `localhost` / 5001 | Command API address (HTTPS + gRPC); `ipc_address` is the deprecated alias of `cmd_host` |
| `agent_ipc.host` / `agent_ipc.port` | `localhost` / 0 (disabled) | Dedicated mTLS IPC server for `/config/v1` (configsync source) |
| `agent_ipc.config_refresh_interval` | 0 (disabled) | configsync poll period (seconds) in sub-processes |
| `agent_ipc.use_socket` / `agent_ipc.socket_path` | false / `${run_path}/agent_ipc.socket` | Unix-socket transport for the IPC server |
| `fleet_policies_dir` | "" | Fleet Automation policy directory (`DD_FLEET_POLICIES_DIR`; Windows registry fallback) |
| `infrastructure_mode` | `full` | `full` / `basic` / `end_user_device` / `cloud_cost_only` / `none` — applies `SourceInfraMode` overrides |
| `autoconfig_from_environment` | true | Enables feature detection; `autoconfig_exclude_features` / `autoconfig_include_features` filter the result |
| `proxy.http` / `proxy.https` / `proxy.no_proxy` | — | Proxy settings, also fed from `DD_PROXY_*` and `HTTP(S)_PROXY`/`NO_PROXY` |
| `use_proxy_for_cloud_metadata` | false | Skip adding cloud-metadata IPs to `no_proxy` |
| `fips.enabled` (+ `fips.local_address`, `fips.port_range_start`, ...) | false | Rewrites every intake URL to the local FIPS proxy port map |
| `delegated_auth.*` | — | Cloud-fetched API keys via [`comp/core/delegatedauth`](<<<SRC>>>/comp/core/delegatedauth) |
| `scrubber.additional_keys` | [] | Extra scrubber keys (deprecated alias `flare_stripped_keys`) |

CLI flags: `--cfgpath/-c` (directory or `.yaml` file), `--extracfgpath`, the fleet-policies dir flag, plus per-command `WithCLIOverride` mappings.

## Deployment-mode differences

1. **Per-OS paths** ([`pkg/util/defaultpaths`](<<<SRC>>>/pkg/util/defaultpaths)): conf `/etc/datadog-agent` (Linux), `C:\ProgramData\Datadog` (Windows), `/opt/datadog-agent/etc` (macOS). Defaults declared as `${conf_path}/...` are localized at `BuildSchema`.
1. **Containers**: `env.IsContainerized()` flips computed defaults — `procfs_path=/host/proc`, `container_cgroup_root=/host/sys/fs/cgroup`, `HOST_ETC=/host/etc` (unless `ignore_host_etc`). Configuration is usually delivered entirely via `DD_*` env vars.
1. **ECS Fargate**: `GetPlatformDefault` in [`pkg/config/setup/config.go`](<<<SRC>>>/pkg/config/setup/config.go) supports per-platform default maps where `fargate` and `container` keys take precedence over GOOS keys.
1. **Windows**: fleet policies dir can come from the registry; the MSI can set `cmd_port` and friends via registry-driven install parameters.
1. **Kubernetes**: the [Cluster Agent](../containers/cluster-agent.md) reads `datadog-cluster.yaml`; the DaemonSet agent gains Kubernetes secret-scoping options (see [Secrets management](secrets.md)).

## Gotchas

1. **The env layer is frozen at startup.** `buildEnvVars` snapshots the environment during `BuildSchema`; the config never re-reads it in production. Empty-string env values are treated as unset.
1. **`Set` silently ignores unknown keys** (error log only) and converts values to the declared default's type — setting the string `"true"` on a bool setting stores `true`, with a one-time conversion warning.
1. **Fleet policies outrank env vars; CLI outranks remote config.** Both surprise people debugging precedence; use `agent config get <key> --source` to see every layer's value.
1. **`MergeConfig`/`MergeFleetPolicy` merge straight into `root`, not into a layer tree** — no `OnUpdate` notifications fire, and a later full re-merge (another `ReadInConfig`) drops those values. They are startup-only by design.
1. **Reading before load**: the global singletons exist from `init()`, so early `Get` calls return defaults; nodetreemodel logs `attempt to read key before config is constructed` when the schema is not built yet — a recurring source of subtle init-order bugs, and `env.GetDetectedFeatures` outright panics.
1. **`agent createschema` hijacks the constructor** based on `os.Args[1]` in [`pkg/config/create/new.go`](<<<SRC>>>/pkg/config/create/new.go) — any binary whose first argument is literally `createschema` gets a schema builder instead of a live config.
1. **Two schemas, one warning pass**: `findUnknownEnvVars` treats system-probe's bound env vars as known when loading `datadog.yaml` (and vice versa); a new sub-agent env var must be bound in *some* schema or it warns at every start.
1. **Test isolation**: never touch the globals in tests — use [`pkg/config/mock`](<<<SRC>>>/pkg/config/mock/mock.go) (`mock.New(t)`). `ChangeChecker` ([`pkg/config/setup/config_change_checker.go`](<<<SRC>>>/pkg/config/setup/config_change_checker.go)) fails a package's `TestMain` if tests leak modifications into the global config.

/// note
The generated schema under [`pkg/config/schema/yaml`](<<<SRC>>>/pkg/config/schema/yaml) is intended to become the single source of truth (codegen back to Go exists via `dda inv schema.codegen`), but today the Go declarations still win. Keep `config_template.yaml`, the Go declarations, and the generated schema in sync through the `schema.*` invoke tasks; CI checks guard against drift.
///
