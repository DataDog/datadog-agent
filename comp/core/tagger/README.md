# package `tagger`

The **Tagger** is the central source of truth for client-side entity tagging. It
runs **Collector**s that detect entities and collect their tags. Tags are then
stored in memory (by the **TagStore**) and can be queried by the tagger.Tag()
method. Calling once tagger.Init() after the **config** package is ready is
needed to enable collection.

The package methods use a common **defaultTagger** object, but we can create
a custom **Tagger** object for testing.

The package will implement an IPC mechanism (a server and a client) to allow
other agents to query the **DefaultTagger** and avoid duplicating the information
in their process. Switch between local and client mode will be done via a build flag.

The tagger is also available to python checks via the `tagger` module exporting
the `get_tags()` function. This function accepts the same arguments as the Go `Tag()`
function, and returns an empty list on errors.

## Collector

A **Collector** connects to a single information source and pushes **types.TagInfo**
structs to a channel, towards the **Tagger**. It can either run in streaming
mode, pull or fetchonly mode, depending of what's most efficient for the data source:

### Streamer

The **DockerCollector** runs in stream mode as it collects events from the docker
daemon and reacts to them, sending updates incrementally.

### Puller

The **KubernetesCollector** will run in pull mode as it needs to query and filter a full entity list every time. It will only push
updates to the store though, by keeping an internal state of the latest
revision.

### FetchOnly

The **ECSCollector** does not push updates to the Store by itself, but is only triggered on cache misses. As tasks don't change after creation, there's no need for periodic pulling. It is designed to run alongside DockerCollector, that will trigger deletions in the store.

## TagStore

The **TagStore** reads **types.TagInfo** structs and stores them in a in-memory
cache. Cache invalidation is triggered by the collectors (or source) by either:

* sending new tags for the same `Entity`, all the tags from this `Source`
  will be removed and replaced by the new tags
* sending a **types.TagInfo** with **DeleteEntity** set, all the tags collected for
  this entity by the specified source (but not others) will be deleted when
  **prune()** is called.

## TagCardinality

**types.TagInfo** accepts and store tags that have different cardinality. **TagCardinality** can be:

* **LowCardinality**: in the host count order of magnitude
* **OrchestratorCardinality**: tags that change value for each pod or task
* **HighCardinality**: typically tags that change value for each web request, user agent, container, etc.

## Entity IDs

Tagger entities are identified by a string-typed ID, with one of the following forms:

<!-- NOTE: a similar table appears in comp/core/autodiscovery/README.md; please keep both in sync -->
| *Service*                               | *Tagger Entity*                                                    |
|-----------------------------------------|--------------------------------------------------------------------|
| workloadmeta.KindContainer              | `container_id://<sha>`                                             |
| workloadmeta.KindContainerImageMetadata | `container_image_metadata://<sha>`                                 |
| workloadmeta.KindGardenContainer        | `container_id://<sha>`                                             |
| workloadmeta.KindKubernetesPod          | `kubernetes_pod_uid://<uid>`                                       |
| workloadmeta.KindECSTask                | `ecs_task://<task-id>`                                             |
| CloudFoundry LRP                        | `<processGuid>/<svcName>/<instanceGuid>`  or `<appGuid>/<svcName>` |
| Container runtime or orchestrator       | (none)                                                             |
| Kubernetes Endpoint                     | `kube_endpoint_uid://<namespace>/<name>/<ip>`                      |
| Kubernetes Service                      | `kube_service://<namespace>/<name>`                                |
| SNMP Config                             | config hash                                                        |

## Tagger

The Tagger handles the glue between **Collectors** and **TagStore** and the
cache miss logic. If the tags from the **TagStore** are missing some sources,
they will be manually queried in a block way, and the cache will be updated.

For convenience, the package creates a **defaultTagger** object that is used
when calling the `tagger.Tag()` method.

                   +-----------+
                   | Collector |
                   +---+-------+
                       |
                       |
    +--------+      +--+-------+       +-------------+
    |  User  <------+  Tagger  +-------> IPC handler |
    |packages|      +--+-----^-+       +-------------+
    +--------+         |     |
                       |     |
                    +--v-----+-+
                    | TagStore |
                    +----------+
