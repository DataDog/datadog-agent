# Autodiscovery

-----

Autodiscovery (AD) is the subsystem that decides *what to monitor* in dynamic environments. Static hosts can be covered by YAML files in `conf.d`, but containers, pods, and cloud services appear and disappear constantly, and their connection details (IPs, ports) are only known at runtime. AD solves this by separating configuration into two streams — *templates* (check configurations with placeholders, discovered from files, pod annotations, container labels, KV stores, or the Cluster Agent) and *services* (live entities observed by listeners, mostly backed by [workloadmeta](../containers/workloadmeta.md)) — and continuously reconciling them. When a template's AD identifiers match a service, the template is resolved (its `%%host%%`-style variables replaced with the service's actual data) and the resulting `integration.Config` is pushed to every registered consumer: the [check collector](collector.md), the [logs agent](../pipelines/logs.md), the [JMXFetch scheduler](jmx.md), and on the Cluster Agent the [cluster-check dispatcher](../containers/cluster-checks.md).

The component lives at [`comp/core/autodiscovery`](<<<SRC>>>/comp/core/autodiscovery) and its central type is `AutoConfig`. The high-level shape (adapted from the package [README](<<<SRC>>>/comp/core/autodiscovery/README.md)):

```text
Kubernetes API ──┐
Cluster Agent ───┤                ┌───────────────┐
Static files ────┤                │ Workloadmeta  │
KV stores ───────┤                └──┬─────────┬──┘
Remote config ───┤                   │         │
             ┌───▼─────────────▼─────▼──┐   ┌──▼───────────┐
             │     Config providers     │   │  Listeners   │
             └──────┬─────┬─────────────┘   └──────┬───────┘
                    │     │ templates     services │
                    │     └──────────► ◄───────────┤
                    │                 │            │
                    │      reconcile (configmgr)   │
                    │                 │            │
                    │ non-template    │ resolved   │
                    │ configs         │ configs    │
                    ▼                 ▼            ▼
             ┌─────────────────────────────────────────┐
             │  scheduler.Controller ("metascheduler") │
             └──┬───────┬────────┬────────┬────────┬───┘
                ▼       ▼        ▼        ▼        ▼
             "check"  "jmx"  logs agent  "clusterchecks"  gRPC stream …
```

## Key packages and files

| Path | Purpose |
|---|---|
| [`comp/core/autodiscovery/def/component.go`](<<<SRC>>>/comp/core/autodiscovery/def/component.go) | The `autodiscovery.Component` interface (`AddConfigProvider`, `AddListeners`, `AddScheduler`, `LoadAndRun`, …) |
| [`comp/core/autodiscovery/impl/autoconfig.go`](<<<SRC>>>/comp/core/autodiscovery/impl/autoconfig.go) | `AutoConfig`: owns pollers, listeners, the config manager, and the scheduler controller |
| [`comp/core/autodiscovery/impl/configmgr.go`](<<<SRC>>>/comp/core/autodiscovery/impl/configmgr.go) | `reconcilingConfigManager`: template ↔ service reconciliation |
| [`comp/core/autodiscovery/impl/config_poller.go`](<<<SRC>>>/comp/core/autodiscovery/impl/config_poller.go) | Per-provider poll/stream loop and config diffing |
| [`comp/core/autodiscovery/providers`](<<<SRC>>>/comp/core/autodiscovery/providers) | All config providers; [`names/provider_names.go`](<<<SRC>>>/comp/core/autodiscovery/providers/names/provider_names.go) lists their user-facing and register names |
| [`comp/core/autodiscovery/listeners`](<<<SRC>>>/comp/core/autodiscovery/listeners) | All service listeners; [`service.go`](<<<SRC>>>/comp/core/autodiscovery/listeners/service.go) defines `WorkloadService` and template filtering |
| [`comp/core/autodiscovery/configresolver/configresolver.go`](<<<SRC>>>/comp/core/autodiscovery/configresolver/configresolver.go) | `Resolve(template, service)`: produces the final config |
| [`pkg/util/tmplvar/resolver.go`](<<<SRC>>>/pkg/util/tmplvar/resolver.go) | `%%variable%%` substitution engine (`TemplateResolver`, per-variable getters) |
| [`comp/core/autodiscovery/common/utils`](<<<SRC>>>/comp/core/autodiscovery/common/utils) | Annotation/label grammar parsing ([`annotations.go`](<<<SRC>>>/comp/core/autodiscovery/common/utils/annotations.go), [`pod_annotations.go`](<<<SRC>>>/comp/core/autodiscovery/common/utils/pod_annotations.go), [`container_labels.go`](<<<SRC>>>/comp/core/autodiscovery/common/utils/container_labels.go)) |
| [`comp/core/autodiscovery/scheduler/controller.go`](<<<SRC>>>/comp/core/autodiscovery/scheduler/controller.go) | Workqueue-based fan-out to registered schedulers |
| [`comp/core/autodiscovery/integration/config.go`](<<<SRC>>>/comp/core/autodiscovery/integration/config.go) | `integration.Config`: the currency of the whole system |
| [`cmd/agent/common/autodiscovery.go`](<<<SRC>>>/cmd/agent/common/autodiscovery.go) | `setupAutoDiscovery`: which providers/listeners each Agent registers at startup |
| [`pkg/config/autodiscovery/autodiscovery.go`](<<<SRC>>>/pkg/config/autodiscovery/autodiscovery.go) | Environment/config-driven auto-enablement of providers and listeners |
| [`comp/core/autodiscovery/discoverer`](<<<SRC>>>/comp/core/autodiscovery/discoverer) | Configuration-discovery worker (async probe-based resolution for templates with a `discovery` section) |

## The currency: `integration.Config`

Everything AD produces or consumes is an [`integration.Config`](<<<SRC>>>/comp/core/autodiscovery/integration/config.go): a check or logs configuration with a `Name`, raw YAML `InitConfig` and `Instances`, an optional `LogsConfig`, and matching metadata. Three predicates drive the whole system:

1. `IsTemplate()` — the config has `ADIdentifiers` or `AdvancedADIdentifiers` attached. Templates are not scheduled directly; they wait to be resolved against a matching service. A config can also carry a `cel_selector` ([CEL](https://cel.dev/) expressions compiled in [`matching_program.go`](<<<SRC>>>/comp/core/autodiscovery/integration/matching_program.go)) which either acts as the sole matching mechanism (a synthetic `cel://` AD identifier is injected) or as a secondary filter next to explicit identifiers.
1. `IsCheckConfig()` — non-cluster-check config with at least one instance; these go to check schedulers.
1. `IsLogConfig()` — carries a `logs:` section; these go to the logs agent.

Configs are identified by a murmur3 `Digest()` covering name, instances, init config, AD identifiers, service ID, and more. The digest is how the config manager and scheduler controller deduplicate, diff, and unschedule. A consequence worth internalizing: *any* edit to a config produces a new digest — AD never updates a config in place, it unschedules the old one and schedules the new one.

## Config providers

Config providers produce `integration.Config`s from some source. They are registered in a catalog by name ([`providers.go`](<<<SRC>>>/comp/core/autodiscovery/providers/providers.go)) and instantiated from three inputs merged in [`setupAutoDiscovery`](<<<SRC>>>/cmd/agent/common/autodiscovery.go): the `config_providers` list in `datadog.yaml`, the `extra_config_providers` string list (commonly set via `DD_EXTRA_CONFIG_PROVIDERS`), and automatic detection — [`DiscoverComponentsFromConfig`](<<<SRC>>>/pkg/config/autodiscovery/autodiscovery.go) (config-driven, e.g. `prometheus_scrape.enabled`) plus [`DiscoverComponentsFromEnv`](<<<SRC>>>/pkg/config/autodiscovery/autodiscovery.go) (environment-driven, e.g. a container runtime was detected). The file provider is always added first, reading `conf.d` plus `fleet_policies_dir`.

Each provider implements one of two interfaces, and [`configPoller`](<<<SRC>>>/comp/core/autodiscovery/impl/config_poller.go) drives it accordingly:

1. `CollectingConfigProvider` — pull-based: `Collect(ctx)` returns the full current config set; the poller diffs it against the previous set by `FastDigest` and emits new/removed configs. When registered with `polling: true` it re-collects every `ad_config_poll_interval` (default 10s), first calling `IsUpToDate` so cheap providers can skip work.
1. `StreamingConfigProvider` — push-based: `Stream(ctx)` returns a channel of `integration.ConfigChanges`. The container provider works this way, subscribed to workloadmeta.

The main providers (register name in parentheses when it differs from the display name):

| Provider | Mode | Runs on | Source |
|---|---|---|---|
| [`file`](<<<SRC>>>/comp/core/autodiscovery/providers/file.go) | collect once (re-poll with `autoconf_config_files_poll`) | all | `conf.d/**` YAML files, including `auto_conf.yaml` container templates |
| [`kubernetes-container-allinone`](<<<SRC>>>/comp/core/autodiscovery/providers/container.go) | streaming | node agents | pod annotations + container labels, via workloadmeta events |
| [`cluster-checks`](<<<SRC>>>/comp/core/autodiscovery/providers/clusterchecks.go) (`clusterchecks`) | polling | node agents / CLC runners | dispatched configs pulled from the Cluster Agent API |
| [`endpoints-checks`](<<<SRC>>>/comp/core/autodiscovery/providers/endpointschecks.go) (`endpointschecks`) | polling | node agents | endpoint-check configs pinned to this node, from the Cluster Agent |
| [`kubernetes-services`](<<<SRC>>>/comp/core/autodiscovery/providers/kube_services.go) (`kube_services`) | polling (informer-invalidated) | Cluster Agent | `ad.datadoghq.com/service.*` annotations on Services |
| [`kubernetes-endpoints`](<<<SRC>>>/comp/core/autodiscovery/providers/kube_endpoints.go) (`kube_endpoints`) | polling (informer-invalidated) | Cluster Agent | `ad.datadoghq.com/endpoints.*` annotations (Endpoints or EndpointSlices variants) |
| [`kube_services_file` / `kube_endpoints_file`](<<<SRC>>>/comp/core/autodiscovery/providers/kube_services_file.go) | collect / polling | Cluster Agent | `conf.d` files with `advanced_ad_identifiers` or CEL selectors targeting Services/Endpoints |
| [`prometheus_pods`](<<<SRC>>>/comp/core/autodiscovery/providers/prometheus_pods.go) / [`prometheus_services`](<<<SRC>>>/comp/core/autodiscovery/providers/prometheus_services.go) / [`prometheus_http_sd`](<<<SRC>>>/comp/core/autodiscovery/providers/prometheus_http_sd.go) | streaming (pods) / polling (services, HTTP SD) | node agent / DCA | `prometheus.io/*` annotations → `openmetrics` check templates |
| [`remote-config`](<<<SRC>>>/comp/core/autodiscovery/providers/remote_config.go) (`remote_config`) | polling (invalidated on RC update) | all | integrations pushed via [remote configuration](../configuration/remote-config.md) |
| [`process_log`](<<<SRC>>>/comp/core/autodiscovery/providers/process_log.go) | streaming | node agents | log configs derived from discovered processes |
| [`consul`](<<<SRC>>>/comp/core/autodiscovery/providers/consul.go) / [`etcd`](<<<SRC>>>/comp/core/autodiscovery/providers/etcd.go) / [`zookeeper`](<<<SRC>>>/comp/core/autodiscovery/providers/zookeeper.go) | polling | all | templates under `autoconf_template_dir` (default `/datadog/check_configs`) in the KV store |
| [`cloudfoundry-bbs`](<<<SRC>>>/comp/core/autodiscovery/providers/cloudfoundry.go) | polling | CF cluster agent | BBS API |

Legacy names from old configs are silently rewritten: `kubelet`, `container`, and `docker` providers all become `kubernetes-container-allinone`, and `docker`/`ecs` listeners become `container`.

## Listeners and services

Listeners watch for *services* — live entities checks could run against — and feed them to `AutoConfig` over the `newService`/`delService` channels. A service implements the [`listeners.Service`](<<<SRC>>>/comp/core/autodiscovery/listeners/service.go) interface: identity (`GetServiceID`), matching (`GetADIdentifiers`, `FilterTemplates`), resolution data (`GetHosts`, `GetPorts`, `GetPid`, `GetHostname`, `GetExtraConfig`), tags (`GetTags` via the [tagger](../containers/tagger.md)), readiness (`IsReady`), and exclusion (`HasFilter`, backed by [`comp/core/workloadfilter`](<<<SRC>>>/comp/core/workloadfilter) `container_exclude`/`container_include` rules).

Listeners are registered by name in [`listeners.go`](<<<SRC>>>/comp/core/autodiscovery/listeners/listeners.go) and activated from the `listeners`/`extra_listeners` config plus the same auto-detection as providers: `container` when a container runtime (Docker, containerd, Podman, ECS sidecar) is detected outside Kubernetes, [`kubelet`](<<<SRC>>>/comp/core/autodiscovery/listeners/kubelet.go) on Kubernetes, [`kube_services`](<<<SRC>>>/comp/core/autodiscovery/listeners/kube_services.go)/[`kube_endpoints`](<<<SRC>>>/comp/core/autodiscovery/listeners/kube_endpoints.go) on the Cluster Agent, plus [`snmp`](<<<SRC>>>/comp/core/autodiscovery/listeners/snmp.go), [`process`](<<<SRC>>>/comp/core/autodiscovery/listeners/process.go), [`database-monitoring-aurora`/`-rds`](<<<SRC>>>/comp/core/autodiscovery/listeners/dbm_aurora.go), [`cloudfoundry-bbs`](<<<SRC>>>/comp/core/autodiscovery/listeners/cloudfoundry.go), the always-on [`static config`](<<<SRC>>>/comp/core/autodiscovery/listeners/staticconfig.go) listener, and the [`environment`](<<<SRC>>>/comp/core/autodiscovery/listeners/environment.go) listener (synthesizes `_containerd`-style pseudo-services so `auto_conf.yaml` templates for runtime checks resolve). Listeners that fail to start with a retriable error are retried every 30 seconds ([`retryListenerCandidates`](<<<SRC>>>/comp/core/autodiscovery/impl/autoconfig.go)).

The [`container` listener](<<<SRC>>>/comp/core/autodiscovery/listeners/container.go) is the archetype: it subscribes to workloadmeta container events, applies exclusion filters, and builds a `WorkloadService` whose AD identifiers are computed by `computeContainerServiceIDs`: the `com.datadoghq.ad.check.id` label overrides everything; otherwise the identifiers are the runtime-prefixed entity (`docker://<sha>`), the long image name, and the short image name. For containers in pods, the per-container annotation `ad.datadoghq.com/<container>.check.id` can add a custom identifier, and pod readiness gates check scheduling (overridable with the `ad.datadoghq.com/tolerate-unready: "true"` pod annotation). The listener can also delay service creation until the tagger reports the container's tags as complete, bounded by `ad_tag_completeness_max_wait` (default 0, disabled) — this prevents the first data points of a fast-starting container from missing Kubernetes tags.

/// warning
The `kubelet` and `container` listeners both emit container services, so enabling both double-schedules every container check. The [`incompatibleListeners`](<<<SRC>>>/cmd/agent/common/autodiscovery.go) guard only prevents this for *auto-detected* listeners — nothing stops you from configuring both explicitly.
///

## Reconciliation: the config manager

[`reconcilingConfigManager`](<<<SRC>>>/comp/core/autodiscovery/impl/configmgr.go) is the heart of AD. It maintains `activeConfigs` (all configs from providers, keyed by digest), `activeServices` (all services from listeners, keyed by service ID), two multimaps indexing both by AD identifier, `serviceResolutions` (service → template digest → resolved-config digest), and `scheduledConfigs` (the current output set). Every mutation — new/removed config, new/removed service — funnels into `reconcileService`, which recomputes the expected set of resolutions for one service and emits an `integration.ConfigChanges` diff (schedule this, unschedule that).

Non-template configs skip reconciliation entirely: they are scheduled as-is (after [secret resolution](../configuration/secrets.md) via `decryptConfig`). Template configs are matched to services through the AD-identifier indexes, then the *service* gets a veto through `Service.FilterTemplates`, which implements the override grammar:

1. An empty `check_names` annotation/label on a service drops all file-based templates for it (a way to disable `auto_conf.yaml` defaults per container).
1. A check name declared in annotations/labels overrides a file or instrumentation template with the same name for that service.
1. Instrumentation-CR checks override file checks of the same name.
1. When `logs_config.container_collect_all` is enabled, the synthetic `container_collect_all` logs template is dropped for services that already have a dedicated logs config.

Templates whose service exists but is not ready yet resolve to `ErrServiceNotReady` and are silently retried on the next reconcile. Failed resolutions are recorded in `errorStats` (surfaced in `agent status` and `agent configcheck`) and reported to the health platform. Templates carrying a `discovery` section are not resolved synchronously at all — they are handed to the [discoverer](<<<SRC>>>/comp/core/autodiscovery/discoverer) worker, which runs asynchronous probes and emits changes on a separate channel.

Secrets get special handling: resolution happens at schedule time, and `AutoConfig` subscribes to secret-refresh notifications, unscheduling and rescheduling any active config whose secret value changed (`processRefreshConfig`). For cluster checks, the config manager also records the check-ID mapping between encrypted and decrypted forms, because the Cluster Agent may dispatch configs without resolving secrets (`secret_backend_skip_checks`) while the runner resolves them — producing different check IDs on each side.

## Template resolution and `%%variables%%`

[`configresolver.Resolve`](<<<SRC>>>/comp/core/autodiscovery/configresolver/configresolver.go) copies the template, stamps it with the service's identity (`ServiceID`, `PodNamespace`, image name, metrics/logs exclusion flags from the workload filters), fetches the service's tags from the tagger at `ChecksConfigCardinality` (or the cardinality named by the template's `check_tag_cardinality`), and then substitutes template variables in `init_config`, every instance, and the logs config. Unless the template sets `ignore_autodiscovery_tags: true`, the service tags are injected into each instance's `tags` list via a post-processor on the parsed YAML tree.

Substitution itself lives in [`pkg/util/tmplvar`](<<<SRC>>>/pkg/util/tmplvar/resolver.go). The supported variables, each backed by a `VariableGetter` reading from the service:

| Variable | Resolves to |
|---|---|
| `%%host%%`, `%%host_<network>%%` | Service IP; with multiple networks, the named one, falling back to `bridge`. IPv6 addresses used inside URLs are automatically bracketed (`http://[::1]:80`) |
| `%%port%%`, `%%port_<idx>%%`, `%%port_<name>%%` | Highest exposed port by default, or the port at a sorted index, or by port name |
| `%%pid%%` | Container/process PID |
| `%%hostname%%` | Container hostname |
| `%%env_<VAR>%%` | An environment variable of the **Agent process** (not the container), gated by `ad_allowed_env_vars` and `ad_disable_env_var_resolution` |
| `%%kube_<key>%%`, `%%extra_<key>%%` | Listener-specific extras via `GetExtraConfig` — e.g. `%%kube_namespace%%`, `%%kube_pod_name%%`, `%%kube_pod_uid%%` from the kubelet listener |

Two mechanical details produce most of the surprises. First, because `%` is invalid in unquoted YAML, the resolver replaces `%%` with the per-mille character `‰` before parsing, resolves `‰var_key‰` patterns against the tree, and restores unconsumed `‰` back to `%%` afterwards — a literal `‰` in a config will be mangled. Second, when a YAML string consists of a *single* variable and nothing else, the resolved value is coerced to an integer or boolean when it parses as one — except for `%%env_*%%`, which always stays a string (an env var like `0123456` must not become octal 42798).

## Annotation and label grammar

The parsing lives in [`common/utils`](<<<SRC>>>/comp/core/autodiscovery/common/utils) and is shared by the container provider, the kubelet/container listeners, and the Cluster Agent's service/endpoint providers.

For pods, annotations are keyed by container name (or by the `check.id` override): the preferred **v2** format is a single JSON object listing named checks, and the older **v1** format uses three parallel JSON arrays. The prefix `ad.datadoghq.com/` has a deprecated ancestor `service-discovery.datadoghq.com/`; container labels use `com.datadoghq.ad.` with the same v1/v2 payloads.

```yaml
# v2 (preferred): one annotation per container
ad.datadoghq.com/redis.checks: |
  {
    "redisdb": {
      "init_config": {},
      "instances": [{"host": "%%host%%", "port": "6379"}]
    }
  }

# v1 (legacy): parallel arrays, indexes must line up
ad.datadoghq.com/redis.check_names: '["redisdb"]'
ad.datadoghq.com/redis.init_configs: '[{}]'
ad.datadoghq.com/redis.instances: '[{"host": "%%host%%", "port": "6379"}]'

# logs config (either version)
ad.datadoghq.com/redis.logs: '[{"source": "redis", "service": "cache"}]'
```

The v2 JSON also accepts `logs`, `ignore_autodiscovery_tags`, and `check_tag_cardinality` per check ([`parseChecksJSON`](<<<SRC>>>/comp/core/autodiscovery/common/utils/pod_annotations.go)). On Services and Endpoints (Cluster Agent), the same grammar applies with the pseudo-identifiers `service` and `endpoints` (`ad.datadoghq.com/service.check_names`, `ad.datadoghq.com/endpoints.checks`, …). Malformed annotations do not fail silently: they land in the "Configuration Errors" section of `agent status` and are reported as health-platform issues.

A logs-only annotation still produces a *template* (its AD identifier is the entity), which resolves into a config with only `LogsConfig` set — it creates log sources in the logs agent but no check.

## The scheduler controller and its consumers

The [`scheduler.Controller`](<<<SRC>>>/comp/core/autodiscovery/scheduler/controller.go) (historically "metascheduler") decouples reconciliation from consumers. `ApplyChanges` records the desired state (scheduled/unscheduled) per config digest in a `ConfigStateStore` and enqueues the digest on a Kubernetes-style workqueue; a single worker goroutine compares desired vs. current state and calls `Schedule`/`Unschedule` on every registered scheduler. Rapid flip-flops therefore collapse: if a config is scheduled and unscheduled before the worker gets to it, nothing is delivered. Consumers registering late (with `replayConfigs: true`) synchronously receive all currently-scheduled configs. A health probe declares the controller unhealthy if a single schedule operation blocks for over 5 minutes — a deliberate deadlock detector for misbehaving consumers.

The controller only starts once workloadmeta reports its collectors initialized (bounded by a 10-second backoff wait in [`newAutoConfig`](<<<SRC>>>/comp/core/autodiscovery/impl/autoconfig.go)), so consumers don't receive configs whose services would still be missing tags.

Registered schedulers by name:

| Name | Consumer | Where |
|---|---|---|
| `check` | `CheckScheduler` → the [check collector](collector.md); loads instances via check loaders and runs them | [`pkg/collector/scheduler.go`](<<<SRC>>>/pkg/collector/scheduler.go), registered in [`cmd/agent/subcommands/run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go) |
| `jmx` | Extracts JMX instances and feeds the [JMXFetch](jmx.md) sidecar state | [`pkg/jmxfetch/scheduler.go`](<<<SRC>>>/pkg/jmxfetch/scheduler.go) |
| `logs-agent AD scheduler` | Creates/removes log sources and services in the [logs pipeline](../pipelines/logs.md) | [`pkg/logs/schedulers/ad/scheduler.go`](<<<SRC>>>/pkg/logs/schedulers/ad/scheduler.go) |
| `clusterchecks` | The Cluster Agent's dispatcher — configs marked `cluster_check: true` are *not* run locally but distributed to nodes | [`pkg/clusteragent/clusterchecks/handler.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/handler.go) |
| `configfiles-discovery` | Reports discovered-but-unscheduled config files as telemetry | [`comp/core/configfilesdiscovery`](<<<SRC>>>/comp/core/configfilesdiscovery/impl/scheduler.go) |
| `remote-<uuid>` | Per-client gRPC `AutodiscoveryStreamConfig` stream on the [IPC API](../processes/ipc.md), used by remote processes and diagnostics | [`comp/core/autodiscovery/stream/stream.go`](<<<SRC>>>/comp/core/autodiscovery/stream/stream.go) |
| `check-cmd` | Ephemeral scheduler used by `agent check` to wait for a named config | [`cmd/agent/common/autodiscovery.go`](<<<SRC>>>/cmd/agent/common/autodiscovery.go) |

Each consumer filters for what it understands: the check scheduler ignores logs-only configs and metrics-excluded services, the logs scheduler ignores configs without `LogsConfig` or with the logs filter set, the JMX scheduler picks only JMX instances.

## Cluster checks and endpoints checks handoff

On the Cluster Agent, the `kube_services`/`kube_endpoints` providers emit templates and the `clusterchecks` dispatcher (an AD scheduler like any other) claims configs flagged `cluster_check: true` instead of running them. Node agents and cluster-check runners close the loop from the other side with the [`clusterchecks` config provider](<<<SRC>>>/comp/core/autodiscovery/providers/clusterchecks.go), which identifies itself to the Cluster Agent API (by `clc_runner_id` or hostname), heartbeats, and polls for its dispatched slice of configs, and the [`endpointschecks` provider](<<<SRC>>>/comp/core/autodiscovery/providers/endpointschecks.go), which fetches endpoint-check configs pinned to the local node (endpoint checks run on the node hosting the backing pod). Both providers implement a degraded mode: during a Cluster Agent outage they keep the current configs scheduled for a grace period instead of unscheduling everything. A cluster-check runner is simply an Agent where `clc_runner_enabled: true` and `clusterchecks` is the *only* config provider ([`IsCLCRunner`](<<<SRC>>>/pkg/config/setup/config.go)) — environment-based provider detection is skipped for it. The dispatching logic, failover semantics, and advanced dispatching are covered in [Cluster checks and endpoints checks](../containers/cluster-checks.md).

## Configuration

| Key | Effect |
|---|---|
| `config_providers` | List of `{name, polling, poll_interval, template_dir, …}` provider activations |
| `extra_config_providers` | Extra provider names (polling mode); typically set via `DD_EXTRA_CONFIG_PROVIDERS` |
| `listeners` / `extra_listeners` | Listener activations |
| `autoconfig_from_environment` | Master switch for environment-based provider/listener detection (default true) |
| `ad_config_poll_interval` | Default polling period for collecting providers (10s) |
| `autoconf_config_files_poll` / `..._interval` | Re-read `conf.d` periodically (default off / 60s) |
| `ad_allowed_env_vars` | Allowlist for `%%env_*%%`; **empty means allow all** |
| `ad_disable_env_var_resolution` | Kill switch for `%%env_*%%` |
| `ad_tag_completeness_max_wait` | Max seconds to delay container services waiting for complete tags (0 = disabled) |
| `container_exclude`, `container_include`, `container_exclude_metrics/_logs`, … | Workload filtering applied by listeners and stamped on resolved configs |
| `autoconf_template_dir` | KV-store template root for consul/etcd/zookeeper |
| `fleet_policies_dir` | Additional `conf.d` root injected by [Fleet Automation](../deployment/fleet.md) |
| `prometheus_scrape.enabled` | Auto-enables the Prometheus pod/service providers |
| `cluster_checks.enabled`, `clc_runner_enabled`, `clc_runner_id` | Cluster-check dispatching and runner identity |
| `logs_config.container_collect_all` | Adds the catch-all container logs template |

## Deployment-mode differences

1. **Host install**: only the file provider and the `static config`/`environment` listeners are active; AD degenerates to "read `conf.d` once" (plus SNMP/DBM listeners if configured).
1. **Kubernetes DaemonSet**: environment detection adds the `kubernetes-container-allinone` provider and the `kubelet` listener; the `clusterchecks`/`endpointschecks` providers are typically added via `DD_EXTRA_CONFIG_PROVIDERS` by the Helm chart when cluster checks are enabled.
1. **Docker/ECS EC2 (no Kubernetes)**: same provider, but the `container` listener instead of `kubelet`; templates come from container labels.
1. **ECS Fargate**: the sidecar agent detects ECS sidecar mode and uses the `container` listener against the task's containers.
1. **Cluster Agent**: runs `kube_services`/`kube_endpoints` providers + listeners against the API server, the file provider, and the `clusterchecks` dispatcher as a scheduler; it schedules the [Cluster Agent's own checks](../containers/cluster-agent.md) through the same `check` scheduler.
1. **Cluster-check runner**: `clusterchecks` provider only; no listeners beyond `static config`; uses the remote tagger pointed at the Cluster Agent so resolved configs still get Kubernetes tags.

## Debugging

`agent configcheck` (endpoint `GET /config-check` on the IPC API, `-v` for verbose) prints every resolved and unresolved config with its provider, source, and the instance IDs the collector derived from it, plus resolution warnings and per-provider errors. The same output lands in flares as `config-check.log`. `agent status` shows AD provider errors and template-resolution warnings, and the [telemetry store](<<<SRC>>>/comp/core/autodiscovery/telemetry/telemetry.go) exposes `scheduled_configs` gauges by provider/type and poll-duration histograms. See [Diagnostics and CLI tools](../operations/diagnostics.md).

## Gotchas

1. **`%%env_*%%` reads the Agent's environment**, not the discovered container's — a frequent source of confusion when templating credentials.
1. **An empty `ad_allowed_env_vars` allows everything**; the restriction is opt-in.
1. **Unschedule-before-schedule**: `applyChanges` always processes unschedules first, so a config edit (new digest) restarts the check rather than updating it, and check IDs change with the config digest.
1. **The `‰` character is reserved** by the template-variable engine; a literal `‰` in a config value is corrupted during resolution.
1. **Single-variable coercion**: `port: %%port%%` yields an integer, but `port: "%%port%% "` (any surrounding text) yields a string — checks that strictly type-check instance fields can break on subtle template edits.
1. **Streaming provider backpressure is intentional**: the container provider's output channel is unbuffered so that configs are always processed before the workloadmeta event is acknowledged — guaranteeing templates exist before the corresponding listener service arrives, which avoids a resolve-miss window.
1. **Provider/listener names have legacy aliases** that are rewritten at startup (`docker`/`ecs` listeners → `container`; `kubelet`/`container`/`docker` providers → `kubernetes-container-allinone`); `agent status` shows the new names, which may not match what is written in the config.
1. **Templates for not-ready pods are retried silently** (`ErrServiceNotReady`), so "my check isn't scheduled" on Kubernetes is often just a pod failing its readiness probe; use `ad.datadoghq.com/tolerate-unready` when that is expected.
1. **One provider instance per name**: registering two AD schedulers or two providers with the same name silently replaces/ignores one of them.
1. **`agent check` cannot see every config source** in one-shot mode (notably remote-config-scheduled integrations), because it spins up its own ephemeral AD scheduler in a separate process.
