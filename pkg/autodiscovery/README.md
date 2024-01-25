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

## Config Providers

The [config providers](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/autodiscovery/providers) draw configuration information from many sources

* Kubernetes (for Endpoints and Services, run only on the cluster agent)
* cluster agent (for cluster checks and endpoints checks);
* static files (`conf.d/<integration>.d/conf.yaml`); and
* the workloadmeta service.

The providers extract configuration from entities' tags, labels, etc. in the form of [`integration.Config`](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/autodiscovery/integration#Config) values.

Some configs are "templates", meaning that they must be resolved with a service to generate a full, non-template config.
Other configs are not templates, and simply contain settings and options for the agent.
Specifically, a config is considered a template if it has _AD identifiers_ attached.
These are strings that identify the services to which the config applies.

## Listeners and Services

The [listeners](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners) monitor entities, known as "services" in this package, such as pods, containers, or tasks.

Each service has two entity identifiers: the AD service ID (from `svc.GetServiceID()`) and the Tagger entity (`svc.GetTaggerEntity()`).
These both uniquely identify an entity, but using different syntax.

<!-- NOTE: a similar table appears in comp/core/tagger/README.md; please keep both in sync -->
| *Service*                         | *Service ID*                                                      | *Tagger Entity*                                                    |
|-----------------------------------|-------------------------------------------------------------------|--------------------------------------------------------------------|
| workloadmeta.KindContainer        | `<runtime>://<sha>`                                               | `container_id://<sha>`                                             |
| workloadmeta.KindGardenContainer  | `garden_container://<sha>`                                        | `container_id://<sha>`                                             |
| workloadmeta.KindKubernetesPod    | `kubernetes_pod://<uid>`                                          | `kubernetes_pod_uid://<uid>`                                       |
| workloadmeta.KindECSTask          | `ecs_task://<task-id>`                                            | `ecs_task://<task-id>`                                             |
| CloudFoundry LRP                  | `<processGuid>/<svcName>/<instanceGuid>` or `<appGuid>/<svcName>` | `<processGuid>/<svcName>/<instanceGuid>`  or `<appGuid>/<svcName>` |
| Container runtime or orchestrator | `_<name>` e.g., `_containerd`                                     | (none)                                                             |
| Kubernetes Endpoint               | `kube_endpoint_uid://<namespace>/<name>/<ip>`                     | `kube_endpoint_uid://<namespace>/<name>/<ip>`                      |
| Kubernetes Service                | `kube_service://<namespace>/<name>`                               | `kube_service://<namespace>/<name>`                                |
| SNMP Config                       | config hash                                                       | config hash                                                        |

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
