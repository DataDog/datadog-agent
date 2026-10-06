# Inspect KSM hash sharding

Use `agent ksm-sharding predict` to calculate where a namespace/resource pair
belongs, and `agent ksm-sharding stores` to inspect the stores created by the
running Agent or cluster-check runner.

## Predict ownership

Prediction is offline: it does not start checks, contact Kubernetes, or require
a running Agent. Supply the same shard count and criteria as the KSM instance:

```sh
agent ksm-sharding predict \
  --shard-count 4 \
  --shard-criteria namespace,resource \
  --namespace default \
  --resource core/Pod
```

Resource identity is a version-independent GroupKind, such as `core/Pod`,
`core/Node`, or `apps/Deployment`. This is the key used by the dynamic store;
using the API resource name `pods` instead would produce a different assignment.
Omit `--namespace` for cluster-scoped resources.

If the resource belongs to a `colocate_resources` group, pass the group's first
collector name with `--colocated-resource`. For example, with
`colocate_resources: [[deployments, replicasets]]`, predict ReplicaSet ownership:

```sh
agent ksm-sharding predict \
  --shard-count 4 \
  --shard-criteria namespace,resource \
  --namespace default \
  --resource apps/ReplicaSet \
  --colocated-resource deployments \
  --shard-id 2
```

`--shard-id` additionally reports whether that shard owns the pair. All shard
IDs are zero-based. Use `--json` for machine-readable output.

## Inspect live stores

Run this on the Agent process that hosts the KSM check:

```sh
agent ksm-sharding stores
agent ksm-sharding stores --check-id kubernetes_state_core:example --json
```

The command uses the authenticated local Agent API. It lists each check's state,
shard ID/count and criteria, followed by its existing stores: namespace,
GroupKind, collector, hash key, owning shard and cached object count. The
`<cluster>` namespace denotes cluster-scoped resources. Extended collectors
remain separate entries even when they share a resource and shard.

The inventory snapshots store metadata and counts; it does not scan objects or
metrics. An active shard can legitimately have no stores. A check still
initializing is reported separately. A created store does not prove its list or
watch succeeded, and zero objects can mean either an empty resource or a store
that has not received data yet.

This inventory covers dynamic hash-sharded stores. Checks using eager collection,
including legacy resource-group sharding, are identified as `eager`; their stores
are not inventoried. The command reports only checks in the local process, not
the entire runner fleet.
