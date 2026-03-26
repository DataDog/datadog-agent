# Component `comp/core/autodiscovery`

## Purpose

`comp/core/autodiscovery` (also known as `AutoConfig` or `AC`) manages the lifecycle of integration configurations for dynamic workloads. It discovers running services (containers, pods, ECS tasks, …), matches them against configuration templates, resolves templates into runnable configs, and notifies downstream consumers (check scheduler, log pipeline, etc.) when configs appear or disappear.

Without autodiscovery every integration would require a static `conf.d/<name>/conf.yaml` file. With it, configuration is derived automatically from container labels, Kubernetes annotations, workloadmeta metadata, and config stores such as etcd or Consul.

## Architecture

```
Kubernetes API ─┐
Cluster Agent  ─┤  ┌─────────────────┐     ┌───────────────┐
Static files   ─┼─►│ Config Providers ├────►│               │
WorkloadMeta   ─┘  └─────────────────┘     │  MetaScheduler│─► Scheduler.Schedule()
                                             │               │
                   ┌─────────────────┐     │               │
WorkloadMeta   ───►│   Listeners     ├────►│               │─► Scheduler.Unschedule()
                   └─────────────────┘     └───────────────┘
```

Config providers supply `integration.Config` values (templates or non-templates).
Listeners report "services" (discovered entities).
When a template's AD identifiers match a service, they are resolved together to produce a runnable config.
Non-template configs are scheduled immediately.
The metascheduler dispatches all scheduled configs to registered `Scheduler` subscribers.

## Key Elements

### Component interface (`comp/core/autodiscovery`)

```go
type Component interface {
    // Register a new config provider. shouldPoll enables periodic Collect(); pollInterval is ignored for streaming providers.
    AddConfigProvider(provider types.ConfigProvider, shouldPoll bool, pollInterval time.Duration)

    // Start all providers and listeners; must be called once after all providers/listeners are added.
    LoadAndRun(ctx context.Context)

    // Returns all currently resolved (scheduled) configs.
    GetAllConfigs() []integration.Config

    // Returns template configs that have not yet been resolved to a service.
    GetUnresolvedConfigs() []integration.Config

    // Register additional listener types by config (e.g., "docker", "kubelet").
    AddListeners(listenerConfigs []pkgconfigsetup.Listeners)

    // Register a Scheduler that will receive Schedule/Unschedule callbacks.
    // If replayConfigs is true, the scheduler is immediately called with all currently active configs.
    AddScheduler(name string, s scheduler.Scheduler, replayConfigs bool)
    RemoveScheduler(name string)

    // Diagnostics / status.
    GetAutodiscoveryErrors() map[string]map[string]types.ErrorMsgSet
    GetProviderCatalog() map[string]types.ConfigProviderFactory
    GetTelemetryStore() *telemetry.Store
    GetIDOfCheckWithEncryptedSecrets(checkID checkid.ID) checkid.ID
    GetConfigCheck() integration.ConfigCheckResponse
}
```

### `integration.Config`

The central data type produced by providers and consumed by schedulers:

```go
type Config struct {
    Name                   string           // integration name (e.g., "redis")
    Instances              []Data           // YAML/JSON instance configs
    InitConfig             Data             // init_config block
    LogsConfig             Data             // logs config
    ADIdentifiers          []string         // AD template IDs; non-empty means this is a template
    AdvancedADIdentifiers  []AdvancedADIdentifier
    CELSelector            workloadfilter.Rules
    Provider               string           // provider name that created this config
    ServiceID              string           // set after resolution to a service
    TaggerEntity           string           // tagger entity ID for tag enrichment
    ClusterCheck           bool
    NodeName               string
    Source                 string
    IgnoreAutodiscoveryTags bool
    CheckTagCardinality    string
    MetricsExcluded        bool
    LogsExcluded           bool
}
```

A config is a **template** if `ADIdentifiers` (or `AdvancedADIdentifiers`) is non-empty. Templates are held until a matching service is discovered, then resolved. Non-templates are scheduled immediately.

### Config providers (`comp/core/autodiscovery/providers/types`)

A config provider supplies `integration.Config` values. There are two flavors:

**Collecting (poll-based):**
```go
type CollectingConfigProvider interface {
    ConfigProvider
    Collect(ctx context.Context) ([]integration.Config, error)
    IsUpToDate(ctx context.Context) (bool, error)
}
```

**Streaming:**
```go
type StreamingConfigProvider interface {
    ConfigProvider
    Stream(ctx context.Context) <-chan integration.ConfigChanges
}
```

Built-in providers include:

| Provider | Source |
|---|---|
| `file` | Static `conf.d/` YAML files |
| `container` | Container labels / Kubernetes pod annotations |
| `clusterchecks` | Configs dispatched by cluster agent |
| `endpointschecks` | Endpoint check configs from cluster agent |
| `kube_endpoints` / `kube_endpointslices` | Kubernetes Endpoints/EndpointSlices |
| `kube_services` | Kubernetes Services |
| `kube_crd` | Custom Resource Definitions |
| `prometheus_pods` / `prometheus_services` | Prometheus annotations |
| `etcd` / `consul` / `zookeeper` | K/V stores |
| `cloudfoundry` | Cloud Foundry API |
| `remote_config` | Remote Configuration service |
| `process_log` | Process-based log collection |

Register a provider at runtime:

```go
ac.AddConfigProvider(myProvider, shouldPoll, pollInterval)
```

Or register a factory in the catalog so it is instantiated by name from `datadog.yaml`:

```go
// comp/core/autodiscovery/providers/providers.go
func RegisterProvider(name string, factory types.ConfigProviderFactory)
```

### Scheduler interface (`comp/core/autodiscovery/scheduler`)

Any component that consumes resolved configs implements `Scheduler`:

```go
type Scheduler interface {
    Schedule(configs []integration.Config)
    Unschedule(configs []integration.Config)
    Stop()
}
```

Register with autodiscovery:

```go
ac.AddScheduler("my-scheduler", myScheduler, true /* replay existing configs */)
```

Key scheduler subscribers in the agent:

| Subscriber | Purpose |
|---|---|
| `CheckScheduler` (`pkg/collector`) | Instantiates and runs checks |
| `logs/schedulers/ad` | Starts log collection for discovered services |
| `comp/logs/adscheduler` | Component-wrapped version of the log AD scheduler |

### Listeners and services

Listeners monitor infrastructure and generate "service" records:

```go
type ServiceListener interface {
    Listen(newSvc chan<- listeners.Service, delSvc chan<- listeners.Service)
    Stop()
}
```

Each `Service` has a service ID and a tagger entity:

| Service kind | Service ID | Tagger entity |
|---|---|---|
| Container | `<runtime>://<sha>` | `container_id://<sha>` |
| Kubernetes pod | `kubernetes_pod://<uid>` | `kubernetes_pod_uid://<uid>` |
| ECS task | `ecs_task://<task-id>` | `ecs_task://<task-id>` |
| Kubernetes endpoint | `kube_endpoint_uid://<ns>/<name>/<ip>` | — |
| Kubernetes service | `kube_service://<ns>/<name>` | — |

Built-in listeners include: `docker`, `containerd`, `kubelet`, `kube_endpoints`, `kube_services`, `ecs`, `snmp`, `cloudfoundry`.

### AD identifiers and template resolution

A config template carries AD identifiers — strings that name the services it applies to. When a listener reports a new service, autodiscovery looks for templates whose AD identifiers match that service's identifiers. On a match, template variables in the instance config are expanded (e.g., `%%host%%`, `%%port%%`, `%%env_VAR%%`) and the resolved config is scheduled.

Advanced AD identifiers (`AdvancedADIdentifiers`) and CEL-based selectors (`CELSelector`) allow more expressive matching beyond simple string equality.

## Relationship to other components

### Upstream: workloadmeta

[`comp/core/workloadmeta`](workloadmeta.md) is the primary source of dynamic service information. Autodiscovery's built-in listeners (`docker`, `containerd`, `kubelet`, `ecs`, …) subscribe to the workloadmeta store rather than connecting to container runtimes directly. When a `KindContainer` or `KindKubernetesPod` event arrives, the matching listener generates a `Service` record that is sent into the template-resolution engine.

The workloadmeta streaming config provider also feeds templates (e.g. configs embedded in pod annotations) directly from the store, bypassing the listener path.

### Upstream: config

[`comp/core/config`](config.md) is used to read the `config_providers` list, `listeners` list, provider-specific settings (e.g. polling intervals, etcd/consul endpoints), and the `AD_*` annotation prefix constants defined in [`pkg/util/kubernetes`](../../pkg/util/kubernetes.md).

### Downstream: check scheduler

[`pkg/collector`](../../pkg/collector/collector.md) registers `CheckScheduler` as an autodiscovery `Scheduler` subscriber. `CheckScheduler.Schedule` resolves `integration.Config` objects into `check.Check` instances via its loader chain; `Unschedule` tears them down. The connection is made at agent startup:

```go
cs := collector.InitCheckScheduler(collectorOpt, senderManager, logReceiver, tagger, filterStore)
ac.AddScheduler("check", cs, true)
```

### Downstream: logs pipeline

[`pkg/logs/schedulers`](../../pkg/logs/schedulers.md) provides `ad.New(ac)` — an autodiscovery `Scheduler` subscriber that translates `integration.Config.LogsConfig` blobs into `*sources.LogSource` values and registers them with the log agent's `SourceManager`. The AD scheduler supports file, container, kubernetes, kube_container, process_log, remote_config, and datastreams_live_messages log config providers.

### Tagger

[`comp/core/tagger`](tagger.md) is injected into the autodiscoveryimpl constructor. When a template is resolved, the resulting `integration.Config.TaggerEntity` field is set to the service's tagger entity (e.g. `container_id://abc123`) so that downstream consumers (checks, log sources) can call `tagger.Tag(entityID, cardinality)` to attach infrastructure tags.

## fx Wiring

The autodiscovery component lives in `comp/core/autodiscovery/autodiscoveryimpl` and exposes an `fx.Module()`:

```go
// cmd/agent/subcommands/run/command.go
autodiscoveryimpl.Module()
```

The noop implementation (`comp/core/autodiscovery/noopimpl`) is used in lightweight binaries that do not need dynamic service discovery.

The `autodiscoveryimpl` constructor accepts:

```go
type dependencies struct {
    fx.In
    Lc          fx.Lifecycle
    Config      configComponent.Component
    Log         logComp.Component
    TaggerComp  tagger.Component
    Secrets     secrets.Component
    WMeta       option.Option[workloadmeta.Component]
    FilterStore workloadfilter.Component
    Telemetry   telemetry.Component
}
```

Autodiscovery depends on the tagger (for tag enrichment during template resolution) and optionally on workloadmeta (for the container/pod listeners and the workloadmeta streaming provider).

## Usage

### Receiving configs in a new consumer

```go
type myScheduler struct{}

func (s *myScheduler) Schedule(configs []integration.Config) {
    for _, cfg := range configs {
        // start collecting for cfg
    }
}
func (s *myScheduler) Unschedule(configs []integration.Config) {
    for _, cfg := range configs {
        // stop collecting for cfg
    }
}
func (s *myScheduler) Stop() {}

// Register once, after autodiscovery has started:
ac.AddScheduler("my-scheduler", &myScheduler{}, true)
```

### Adding a static config provider

```go
provider, err := providers.NewFileConfigProvider(...)
ac.AddConfigProvider(provider, true /* poll */, 30*time.Second)
```

### Triggering autodiscovery

```go
ac.LoadAndRun(ctx)
```

This starts all registered providers (polling or streaming) and all configured listeners. Call it once, after all providers and schedulers have been registered.

### Inspecting active configs

```bash
agent configcheck          # shows all resolved and unresolved configs
agent status               # includes autodiscovery section
```

Or programmatically:

```go
for _, cfg := range ac.GetAllConfigs() {
    fmt.Println(cfg.Name, cfg.ServiceID)
}
errors := ac.GetAutodiscoveryErrors()  // per-provider errors
```

### Writing a new config provider

1. Implement `types.CollectingConfigProvider` or `types.StreamingConfigProvider`.
2. Register the factory with `providers.RegisterProvider("my-provider", factory)`.
3. Add `"my-provider"` to `config_providers` in `datadog.yaml`, or call `ac.AddConfigProvider(...)` directly.

### Template variables

When writing config templates (in container labels or `conf.d/` files), the following variables are expanded at resolution time:

| Variable | Value |
|---|---|
| `%%host%%` | Service's primary IP |
| `%%port%%` | Last exposed port (lowest-to-highest order) |
| `%%port_<n>%%` | Nth exposed port |
| `%%env_<VAR>%%` | Container environment variable |
| `%%hostname%%` | Container hostname |
| `%%pid%%` | Container PID |
| `%%kube_namespace%%` | Pod namespace |

Full reference: https://docs.datadoghq.com/agent/guide/template_variables/

Container labels and Kubernetes pod annotations used as AD template sources rely on the annotation key constants in [`pkg/util/kubernetes`](../../pkg/util/kubernetes.md): `ADAnnotationPrefix` (`ad.datadoghq.com/`), `ADTagsAnnotation`, and `ADContainerTagsAnnotationFormat`. Unified Service Tagging labels (`EnvTagLabelKey`, `ServiceTagLabelKey`, `VersionTagLabelKey`) are resolved by the tagger (see [`comp/core/tagger`](tagger.md)) rather than by autodiscovery itself.
