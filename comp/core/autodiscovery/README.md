# package `autodiscovery`

This package manages configuration for dynamic entities like pods and containers.

## Architecture

The high-level architecture of this package is implemented by the AutoConfig type, and looks like this:

```
Kubernetes
    API──────┐
             │
Cluster      │ ┌────────────────┐
  Agent────┐ │ │ Workload Meta  │
           │ │ └──┬──────────┬──┘
 Static    │ │    │          │
  Files──┐ │ │    │          │
         │ │ │    │          │
    ┌────▼─▼─▼────▼──┐    ┌──▼─────────────┐
    │Config Providers│    │   Listeners    │
    └──┬─────┬───────┘    └────────┬───────┘
       │     │                     │
       │     │                     │
       │     │templates    services│
       │     └────────► │ ◄────────┤
       │                │          │
       │            reconcile      │
       │                │          │
       │non-template    │          │
       │configs         │          │
       │                │          │
       │                ▼          │
       │         ┌─────────────┐   │
       └────────►│metascheduler│◄──┘
                 └─────────────┘
```

## Synchronous startup tracing

With `DD_FX_TRACING_ENABLED=true`, the core Agent records AD setup phases in the
existing `dd-agent-fx-init` trace. Initialization remains synchronous: these spans
are children of the original `LoadComponents` Fx startup hook, not a new
background preparation operation.

Phase names identify file-reader initialization (including its initial scan,
parsing, and cache fill), configuration/environment discovery, provider/listener
configuration, and each registered provider factory. Listener initialization has
nested spans for candidate registration, lock acquisition, each factory attempt,
and each `Listen` call. Resources are fixed phase labels or registered
provider/listener types; no configuration values, paths, URLs, or error messages
are recorded. A factory error marks its phase as failed without changing Fx's
startup outcome or the existing retry policy.

The recorder is app-local and is supplied by `DefaultFxLoggingOption`. It records
only inside synchronous `OnStart` hooks. Listener retries are intentionally
excluded, and late phase completions are discarded rather than attached to a
later hook. If startup ends while work is still running, its partial hook/phase
spans are retained with `startup.incomplete=1`; these are elapsed times at the
snapshot boundary, not completed operation durations. This also preserves the
parent chain of phases that already finished. It captures at most 128 phases per
startup and 32 finite numeric metrics per phase; any cap-related span omissions
are reported in the root span's `startup.phase_spans_dropped` metric. Disabled or
absent recorders do no I/O and allocate no phase objects.

These timings distinguish expensive setup work from other startup hooks. They do
not measure first provider collection, workloadmeta readiness, first check
execution, or end-to-end telemetry delivery. Asynchronous preparation would need
a different recorder lifetime; it must not reuse this synchronous hook scope.

### File-reader breakdown

`autodiscovery.config_files.initialize` has the following child spans, all with
resource `file`:

- `autodiscovery.config_files.enumerate`: top-level `ReadDir`, once per search path.
- `autodiscovery.config_files.read_parse`: worker-pool wall time for that path,
  including dispatch, nested `.d` enumeration, file reads, parsing/processing,
  joining workers, and collecting their counters.
- `autodiscovery.config_files.merge`: ordered result merging for that path.
- `autodiscovery.config_files.merge_defaults`: final default/override selection.
- `autodiscovery.config_files.cache_publish`: publishing the three cache entries.

Per-path spans carry numeric `config_files.path_index`, never the path itself.
An absent optional path has `config_files.path_missing=1`, not an error flag.
`config_files.initial_scan` on the initializer is 1 when this call performs the
one-time scan, or 0 when initialization has already happened. Later cache refreshes
are not traced with this startup phase.

The initializer and read/parse-pool spans include aggregate metrics:

| Metric suffix (under `config_files.`) | Meaning |
|---|---|
| `files_attempted`, `files_read`, `read_errors`, `bytes_read` | Actual YAML file read attempts, successful reads (including empty files), failures, and bytes returned |
| `empty_files`, `configs_parsed`, `config_errors` | Empty reads, successfully processed nonempty configs, and parsing/processing failures |
| `nested_directories_read`, `nested_directory_errors` | Successful/failed `.d` directory enumeration inside workers |
| `nested_directory_read_ns`, `file_read_ns`, `parse_ns` | **Summed per-worker elapsed nanoseconds**, not CPU time or separate wall-clock spans |

`parse_ns` covers validation, YAML unmarshalling/marshalling, scrubbing, and
configuration processing after reading a nonempty file. Because workers overlap,
these duration sums can exceed the read/parse span's wall duration; do not subtract
them from that duration or treat them as CPU utilization. More profiling is needed
to distinguish computation, scheduling, and lock waits.

The pool also records `workers_configured`, `workers_used` (after clamping), and
`entries` (top-level entries, including ones later skipped). The initializer
records total `entries`, `search_paths`, final `configs_loaded`, `config_formats`,
and `integration_errors`. Successfully parsed defaults that are overridden, or
root-level metric files that are discarded, still count as parsed but not loaded.
Default selection records `defaults_seen` and `defaults_added`. Parsing failures
remain best-effort, as before; inspect the counters rather than interpreting a
successful batch span as every file being valid.

Counters are private to each worker and reduced after joining, with no per-file
spans, recorder locks in the worker loop, or retained filenames/content. Worker
counter allocation and per-file timing are skipped when initialization is called
without a tracing phase.

## Config Providers

The [config providers](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers) draw configuration information from many sources

* Kubernetes (for Endpoints and Services, run only on the cluster agent)
* cluster agent (for cluster checks and endpoints checks);
* static files (`conf.d/<integration>.d/conf.yaml`); and
* the workloadmeta service.

The providers extract configuration from entities' tags, labels, etc. in the form of [`integration.Config`](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration#Config) values.

Some configs are "templates", meaning that they must be resolved with a service to generate a full, non-template config.
Other configs are not templates, and simply contain settings and options for the agent.
Specifically, a config is considered a template if it has _AD identifiers_ attached.
These are strings that identify the services to which the config applies.

## Listeners and Services

The [listeners](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners) monitor entities, known as "services" in this package, such as pods, containers, or tasks.

Each service has two entity identifiers: the AD service ID (from `svc.GetServiceID()`) and the Tagger entity (`svc.GetTaggerEntity()`).
These both uniquely identify an entity, but using different syntax.

<!-- NOTE: a similar table appears in comp/core/tagger/README.md; please keep both in sync -->
| *Service*                         | *Service ID*                                                      | *Tagger Entity*              |
|-----------------------------------|-------------------------------------------------------------------|------------------------------|
| workloadmeta.KindContainer        | `<runtime>://<sha>`                                               | `container_id://<sha>`       |
| workloadmeta.KindKubernetesPod    | `kubernetes_pod://<uid>`                                          | `kubernetes_pod_uid://<uid>` |
| workloadmeta.KindECSTask          | `ecs_task://<task-id>`                                            | `ecs_task://<task-id>`       |
| CloudFoundry LRP                  | `<processGuid>/<svcName>/<instanceGuid>` or `<appGuid>/<svcName>` | (none)                       |
| Container runtime or orchestrator | `_<name>` e.g., `_containerd`                                     | (none)                       |
| Kubernetes Endpoint               | `kube_endpoint_uid://<namespace>/<name>/<ip>`                     | (none)                       |
| Kubernetes Service                | `kube_service://<namespace>/<name>`                               | (none)                       |
| SNMP Config                       | config hash                                                       | (none)                       |

## MetaScheduler

The metascheduler handles notifying consumers of new or removed configs.
It can notify in three circumstances:

1. When a config provider detects a non-template configuration, that is published immediately by the metascheduler.
2. Whenever template configurations or services change, these are reconciled by matching AD identifiers, any new or removed configs are published by the metascheduler.
3. For every service, a "service config" -- one with no provider and no configuration -- is published by the metascheduler.
   Only service configs have an entity defined.

## Resolving Templates

Entities that contain their own configuration are reconciled using an AD identifier unique to that entity.
For example, a new container might be detected first by a listener, creating a new service with an AD identifier containing its SHA.
Soon after, the relevant config provider detects the container, extracts configuration from its labels, and creates an `integration.Config` containing the same AD identifier.

The reconciliation process combines the service and the Config, resolving the template, and schedules the resolved config.
In the process, [template variables](https://docs.datadoghq.com/agent/faq/template_variables/) are expanded based on values from the service.
The resulting config is then scheduled with the MetaScheduler.
