# package `tagger`

The **Tagger** is the central source of truth for client-side entity tagging. It
subscribes to workloadmeta to get updates for all the entity kinds (containers,
kubernetes pods, kubernetes nodes, etc.) and extracts the tags for each of them.
Tags are then stored in memory (by the **TagStore**) and can be queried by the
`tagger.Tag()` method. Calling once `tagger.Start()` after the **config**
package is ready is needed to enable collection.

The package methods use a common **defaultTagger** object, but we can create
a custom **Tagger** object for testing.

The package implements an IPC mechanism (a server and a client) to allow other
agents to query the **DefaultTagger** and avoid duplicating the information in
their process. Check the `remote` package for more details.

The tagger is also available to python checks via the `tagger` module exporting
the `get_tags()` function. This function accepts the same arguments as the Go `Tag()`
function, and returns an empty list on errors.

## Workloadmeta

The entities that need to be tagged are collected by workloadmeta. The tagger
subscribes to workloadmeta to get updates for all the entity kinds (containers,
kubernetes pods, kubernetes nodes, etc.) and extracts the tags for each of them.

## TagStore

The **TagStore** reads **types.TagInfo** structs and stores them in an in-memory
cache. Cache invalidation is triggered by the collectors (or source) by either:

* sending new tags for the same `Entity`, all the tags from this `Source`
  will be removed and replaced by the new tags.
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
| *Entity*                                | *ID*                                                                                                                 |
|-----------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| workloadmeta.KindContainer              | `container_id://<sha>`                                                                                               |
| workloadmeta.KindContainerImageMetadata | `container_image_metadata://<sha>`                                                                                   |
| workloadmeta.KindECSTask                | `ecs_task://<task-id>`                                                                                               |
| workloadmeta.KindKubernetesDeployment   | `deployment://<namespace>/<name>`                                                                                    |
| workloadmeta.KindKubernetesMetadata     | `kubernetes_metadata://<group>/<resourceType>/<namespace>/<name>` (`<namespace>` is empty in cluster-scoped objects) |
| workloadmeta.KindKubernetesPod          | `kubernetes_pod_uid://<uid>`                                                                                         |
| workloadmeta.KindProcess                | `process://<pid>`                                                                                                    |

## Tagger

The Tagger handles the glue between the workloadmeta collector and the
**TagStore**.

For convenience, the package creates a **defaultTagger** object that is used
when calling the `tagger.Tag()` method.

                   +--------------+
                   | Workloadmeta |
                   +---+----------+
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
