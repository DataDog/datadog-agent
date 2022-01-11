# package `autodiscovery`

This package manages configuration for dynamic entities like pods and containers.

## Architecture

The high-level architecture of this package is implemented by the AutoConfig type, and looks like this:

```
               ┌────────────────┐
               │ Workload Meta  │
               └──┬──────────┬──┘
  Static          │          │
  Files──┐        │          │
         │        │          │
    ┌────▼────────▼──┐    ┌──▼─────────────┐
    │Config Providers│    │   Listeners    │
    └──┬─────┬───────┘    └────────┬───────┘
       │     │                     │
       │     │templates    services│
       │     │                     │
       │     └────► reconcile ◄────┤
       │                │          │
       │                │          │
       │                │          │
       │non-template    │          │
       │configs         │          │
       │                │          │
       │                ▼          │
       │         ┌─────────────┐   │
       └────────►│metascheduler│◄──┘
                 └─────────────┘
```

The [config providers](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/autodiscovery/providers) draw configuration information from many sources, including static files (`conf.d/<integration>.d/conf.yaml`) and the workloadmeta service.
The providers extract configuration from entities' tags, labels, etc. in the form of [`integration.Config`](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/autodiscovery/integration#Config) values.

Some configs are "templates", meaning that they must be resolved with a service to generate a full, non-template config.
Other configs are not templates, and simply contain settings and options for the agent.
Specifically, a config is considered a template if it has _AD identifiers_ attached.
These are strings that identify the services to which the config applies.

The [listeners](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners) monitor entities, known as "services" in this package, such as pods, containers, or tasks.

The metascheduler handles notifying consumers of new or removed configs.
It can notify in three circumstances:

1. When a config provider detects a non-template configuration, that is published immediately by the metascheduler.
2. Whenever template configurations or services change, these are reconciled by matching AD identifiers, any new or removed configs are published by the metascheduler.
3. For every service, a "service config" -- one with no provider and no configuration -- is published by the metascheduler.

Entities that contain their own configuration are reconciled using an AD identifier unique to that entity.
For example, a new container might be deteted first by a listener, creating a new serivce with an AD identifier containing its SHA.
Soon after, the relevant config provider detects the container, extracts configuration from its labels, and creates an `integration.Config` containing the same AD identifier.
The reconciliation process combines the service and the Config, resolving the template, and schedules the resolved config.
