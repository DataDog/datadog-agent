# Workloadmeta Store

This package is responsible for gathering information about workloads and disseminating that to other components.

## Entities

An _Entity_ represents a single unit of work being done by a piece of software, like a process, a container, a Kubernetes pod, or a task in any cloud provider, that the agent would like to observe.
The current workload of a host or cluster is represented by the current set of entities.

Each _Entity_ has a unique _EntityID_, composed of a _Kind_ and an ID.
Examples of kinds include container, pod, and task.
The full list is in the documentation for the `Kind` type.

Note that in this package, entities are always identified by EntityID (kind and ID), not with a URL-like string.

## Sources

The Workloadmeta Store monitors information from various _sources_.
Examples of sources include container runtimes and various orchestrators.
The full list is in the documentation for the `Source` type.

Multiple sources may generate events about the same entity.
When this occurs, information from those sources is merged into one entity.

## Store

The _Store_ is the central component of the package, storing the set of entities.
A store has a set of _collectors_ responsible for notifying the store of workload changes.
Each collector is specialized to a particular external service such as Kubernetes or ECS, roughly corresponding to a source.
Collectors can either poll for updates, or translate a stream of events from the external service, as appropriate.

The store provides information to other components either through subscriptions or by querying the current state.

### Subscription

Subscription provides a channel containing event bundles.
Each event in a bundle is either a "set" or "unset".
A "set" event indicates new information about an entity -- either a new entity, or an update to an existing entity.
An "unset" event indicates that an entity no longer exists.
The first event bundle to each subscriber contains a "set" event for each existing entity at that time.
It's safe to assume that this first bundle corresponds to entities that existed before the agent started.

## Telemetry and Debugging

The Workloadmeta Store produces agent telemetry measuring the behavior of the component.
The metrics are defined in `comp/core/workloadmeta/telemetry/telemetry.go`

The `agent workload-list` command will print the workload content of a running agent.

The code in `comp/core/workloadmeta/dumper` logs all events verbosely, and may be useful when debugging new collectors.
It is not built by default; see the comments in the package for how to set it up.
