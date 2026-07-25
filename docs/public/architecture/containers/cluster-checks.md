# Cluster checks and endpoints checks

-----

Some checks target cluster-wide entities — a Kubernetes service, an external database's load balancer, the kube-state-metrics feed — and must run exactly once per cluster rather than once per node. Cluster checks solve this: the [Cluster Agent](cluster-agent.md) (DCA) leader discovers the check configurations through its own [Autodiscovery](../checks/autodiscovery.md), stamps them with cluster-level tags, and dispatches each one to exactly one node agent or dedicated cluster-check runner (CLC runner), which polls the DCA for its assignments. Endpoints checks are the per-pod sibling: configurations resolved from Kubernetes Endpoints objects that must run on the node hosting the backing pod.

The dispatching logic lives in [`pkg/clusteragent/clusterchecks`](<<<SRC>>>/pkg/clusteragent/clusterchecks); the node-agent side is a pair of Autodiscovery config providers. The in-repo [`README.md`](<<<SRC>>>/pkg/clusteragent/clusterchecks/README.md) has a shorter tour of the same code.

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/clusteragent/clusterchecks/handler.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/handler.go) | `Handler`: leadership state machine, warmup, dispatch lifecycle |
| [`pkg/clusteragent/clusterchecks/dispatcher_main.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_main.go) | `dispatcher`: AD scheduler implementation, `Schedule`/`Unschedule`, config patching, cleanup loop |
| [`pkg/clusteragent/clusterchecks/dispatcher_nodes.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_nodes.go) | Node bookkeeping: heartbeats, `processNodeStatus`, node choice, expiry |
| [`pkg/clusteragent/clusterchecks/dispatcher_configs.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_configs.go) | Config store operations: add, remove, reassign |
| [`pkg/clusteragent/clusterchecks/dangling_config.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dangling_config.go) | Dangling (unassigned) config tracking and the unscheduled-check flag |
| [`pkg/clusteragent/clusterchecks/dispatcher_rebalance.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_rebalance.go) | Busyness- and utilization-based rebalancing across CLC runners |
| [`pkg/clusteragent/clusterchecks/dispatcher_isolate.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_isolate.go) | Isolating one check onto its own runner |
| [`pkg/clusteragent/clusterchecks/dispatcher_endpoints_configs.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_endpoints_configs.go) | Per-node storage of endpoints-check configs |
| [`pkg/clusteragent/clusterchecks/ksm_sharding.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/ksm_sharding.go), [`dispatcher_ksm.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_ksm.go) | Sharding the `kubernetes_state_core` check across runners |
| [`pkg/clusteragent/clusterchecks/stores.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/stores.go) | `clusterStore`: digest→node, digest→config, per-node stores, dangling pool |
| [`cmd/cluster-agent/api/v1/clusterchecks.go`](<<<SRC>>>/cmd/cluster-agent/api/v1/clusterchecks.go), [`endpointschecks.go`](<<<SRC>>>/cmd/cluster-agent/api/v1/endpointschecks.go) | The HTTP endpoints node agents poll |
| [`comp/core/autodiscovery/providers/clusterchecks.go`](<<<SRC>>>/comp/core/autodiscovery/providers/clusterchecks.go) | Node-agent AD provider: status heartbeat + config fetch |
| [`comp/core/autodiscovery/providers/endpointschecks.go`](<<<SRC>>>/comp/core/autodiscovery/providers/endpointschecks.go) | Node-agent AD provider for endpoints checks (fetch by node name) |
| [`pkg/util/clusteragent/clusterchecks.go`](<<<SRC>>>/pkg/util/clusteragent/clusterchecks.go), [`clcrunner.go`](<<<SRC>>>/pkg/util/clusteragent/clcrunner.go) | Node-agent-side clients: DCA polling, CLC-runner stats client |

## End-to-end flow

```text
   DCA leader                                       node agents / CLC runners

 Autodiscovery (file cluster_check:true,
  kube_services, kube_endpoints, CRD, prometheus)
        |
        v  integration.Config{ClusterCheck: true}
   Handler (leadership state machine, warmup)
        |
        v  AddScheduler("clusterchecks", dispatcher)
   dispatcher.Schedule
        |-- NodeName set --> endpoints-config store --- GET /api/v1/endpointschecks/configs/{nodeName}
        |-- KSM + sharding --> shard configs
        `-- else: patchConfiguration + assign node
                 |
                 v
           clusterStore (digest -> node)  <--- POST /api/v1/clusterchecks/status/{id}   (10 s heartbeat)
                                          ---> GET  /api/v1/clusterchecks/configs/{id}  (when out of date)
```

## The handler state machine

The `Handler` (created by `NewHandler`, run in its own goroutine) tracks three states — `unknown`, `leader`, `follower` — driven by a 1-second leadership watch (`leaderStatusFreq`) that calls `GetLeaderIP()` on the leader-election engine. An empty leader IP means "I am the leader". State transitions are pushed into `leadershipChan`; the same callback updates the `LeaderForwarder`'s target IP so follower replicas can proxy node-agent queries to the leader. When `leader_election` is disabled, the handler assumes permanent single-replica leadership.

On becoming leader, the handler does **not** dispatch immediately:

1. It waits `cluster_checks.warmup_duration` (30 s) so node agents can re-register after the leadership change. During warmup, `processNodeStatus` deliberately answers `isUpToDate: true` to every polling agent, keeping their cached checks running instead of tearing them down.
1. After warmup it calls `UpdateAdvancedDispatchingMode` (see below), registers the `dispatcher` as an Autodiscovery scheduler named `clusterchecks` with a config replay, and runs the dispatch loop until leadership is lost.
1. On losing leadership, the dispatch context is canceled; `runDispatch` calls `RemoveScheduler` **before** `dispatcher.reset()`. The ordering closes a race: if Autodiscovery fires a `Schedule` call between `reset()` clearing the KSM shard state and `RemoveScheduler` stopping new calls, the shard map is repopulated and the next leadership term silently drops the KSM check.

Losing leadership never restarts the process — dispatch state is rebuilt from the AD replay on the next term, behind a fresh warmup.

## Dispatching

`dispatcher.Schedule` in [`dispatcher_main.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_main.go) classifies each incoming `integration.Config` with `ClusterCheck: true`:

- Checks named in `cluster_checks.exclude_checks`, or filtered by container filtering, are skipped.
- Configs carrying a `NodeName` are **endpoints checks**, stored per node (below), not dispatched.
- `kubernetes_state_core` configs may be **sharded** (below).
- Everything else is patched by `patchConfiguration`: the `ClusterCheck` flag is cleared (so the receiving agent schedules it as a normal check), an explicit empty hostname is set (cluster-level data should not inherit the runner's hostname), and `extraTags` are injected — `cluster_name`/`kube_cluster_name`, `orch_cluster_id`, `cluster_checks.extra_tags`, and the DCA tagger's global tags.

The patched config is then assigned to a node by `getNodeToScheduleCheck` in [`dispatcher_nodes.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_nodes.go). With advanced dispatching the target is a **random** node — runner statistics are only refreshed at rebalance time, so "least busy" would pile every new check onto the same node between rebalances. Without advanced dispatching, the node with the fewest checks wins.

Configs that cannot be assigned (no nodes registered yet) go to the **dangling** pool and are retried on every cleanup tick; a config dangling longer than `cluster_checks.unscheduled_check_threshold` (60 s) is flagged as unscheduled in telemetry ([`dangling_config.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dangling_config.go)). Nodes that miss heartbeats for `cluster_checks.node_expiration_timeout` (30 s) are expired by `expireNodes`, and their configs move back to dangling for redispatch.

All of this state lives in the `clusterStore` ([`stores.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/stores.go)): digest→node and digest→config maps, per-node stores with heartbeat timestamps, the endpoints-config map keyed by node name, and the dangling pool.

## The node-agent side

Agents participate by enabling the `clusterchecks` config provider. `ClusterChecksConfigProvider` in [`clusterchecks.go`](<<<SRC>>>/comp/core/autodiscovery/providers/clusterchecks.go) runs a poll loop against the DCA:

1. `POST /api/v1/clusterchecks/status/{identifier}` every ~10 s, carrying the last-seen configuration version (`LastChange`). The identifier is `clc_runner_id` if set, otherwise the hostname. The reply says whether the agent is up to date.
1. When out of date, `GET /api/v1/clusterchecks/configs/{identifier}` fetches the full assigned config set, which the provider feeds into local Autodiscovery like any other config source.
1. A background `heartbeatSender` sends an extra status POST (with the sentinel `ExtraHeartbeatLastChangeValue`, which only refreshes the heartbeat) whenever the main loop risks blowing past `node_expiration_timeout` — protecting slow polls from getting the node expired and its checks redispatched.

Both endpoints are leader-only; followers proxy them via the `LeaderForwarder`, and an `unknown`-state DCA answers 503 "Startup in progress", which the provider retries.

## CLC runners and advanced dispatching

A **cluster-check runner** is a regular agent deployed with `clc_runner_enabled: true` and only the `clusterchecks` config provider (detected by `pkgconfigsetup.IsCLCRunner`). Runners report `NodeType: NodeTypeCLCRunner` in their status POSTs, along with their pod IP in the `X-Real-Ip` header — the DCA needs it to call back.

With `cluster_checks.advanced_dispatching_enabled` (default true), the DCA leader queries each runner's own API — `GET /api/v1/clcrunner/stats` on `cluster_checks.clc_runners_port` (5005), client in [`clcrunner.go`](<<<SRC>>>/pkg/util/clusteragent/clcrunner.go) — to compute per-runner *busyness* from check execution times. Every `cluster_checks.rebalance_period` (10 min) the dispatcher rebalances ([`dispatcher_rebalance.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_rebalance.go)): a check moves only if `destBusyness + checkWeight < srcBusyness * 0.9`. With `cluster_checks.rebalance_with_utilization` (default true) placement instead minimizes predicted worker utilization, with optional stickiness (`cluster_checks.stickiness_*`) to damp oscillation.

Operational levers: `POST /api/v1/clusterchecks/rebalance` (the `datadog-cluster-agent clusterchecks rebalance` CLI) forces a rebalance, and `POST /api/v1/clusterchecks/isolate/check/{id}` ([`dispatcher_isolate.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_isolate.go)) evacuates everything else off a runner so one heavy check gets it exclusively.

/// warning
Advanced dispatching silently degrades: if a single plain node agent (non-CLC) joins the cluster-check pool, `processNodeStatus` disables busyness-based dispatching for the remainder of the leadership term. Mixed pools fall back to check-count balancing with no error — only a log line and telemetry reveal it.
///

## Endpoints checks

The `kube_endpoints` AD provider on the DCA ([`kube_endpoints.go`](<<<SRC>>>/comp/core/autodiscovery/providers/kube_endpoints.go)) resolves annotated Kubernetes services into one config per endpoint address. Configs backed by a pod carry that pod's `NodeName` and are stored by the dispatcher in the per-node endpoints map ([`dispatcher_endpoints_configs.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/dispatcher_endpoints_configs.go)); the node agent running on that node fetches them with `GET /api/v1/endpointschecks/configs/{nodeName}` through the `endpointschecks` provider ([`endpointschecks.go`](<<<SRC>>>/comp/core/autodiscovery/providers/endpointschecks.go)) — keyed by the *Kubernetes node name*, not the cluster-check identifier. Running the check on the pod's own node preserves network locality and lets the agent tag with local container context.

Endpoint addresses not backed by pods (for example, ClusterIP services or external endpoints) cannot be pinned to a node and are dispatched as normal cluster checks instead. Endpoints checks bypass the dispatch/rebalance state machine entirely: no heartbeat accounting, no dangling pool, and `LastChange: 0` semantics on the fetch path. The feature requires the endpoints/endpointslices informers, registered when `cluster_checks.enabled` is set.

## KSM sharding

`kubernetes_state_core` is usually the heaviest cluster check. With `cluster_checks.ksm_sharding_enabled` (default false, requires advanced dispatching), [`ksm_sharding.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/ksm_sharding.go) splits one KSM config into shards by resource group — {pods}, {nodes}, and {everything else} — so multiple CLC runners share the load, each shard a separate dispatched config.

Sharding breaks check-level cross-resource features: `labels_as_tags` and `label_joins` that join across resource types cannot work when the resources land on different runners. The global `kubernetes_resources_labels_as_tags` / `kubernetes_resources_annotations_as_tags` settings must be used instead. See [Go core checks](../checks/corechecks.md) for the KSM check itself and [Orchestrator explorer](orchestrator.md) for its collector-list relationship with the orchestrator check.

## Configuration

| Key | Default | Meaning |
|---|---|---|
| `cluster_checks.enabled` | false | Master switch on the DCA |
| `cluster_checks.warmup_duration` | 30 s | Wait after gaining leadership before dispatching |
| `cluster_checks.node_expiration_timeout` | 30 s | Heartbeat timeout before a node's checks are redispatched |
| `cluster_checks.unscheduled_check_threshold` | 60 s | Dangling duration before a config is flagged unscheduled (clamped to ≥ `node_expiration_timeout`) |
| `cluster_checks.extra_tags` | — | Extra tags stamped onto every dispatched config |
| `cluster_checks.advanced_dispatching_enabled` | true | Busyness-based dispatching using CLC-runner stats |
| `cluster_checks.rebalance_period` | 10 min | Rebalance interval |
| `cluster_checks.rebalance_with_utilization` | true | Use worker-utilization placement instead of raw busyness |
| `cluster_checks.clc_runners_port` | 5005 | Port the DCA uses to query runner stats |
| `cluster_checks.exclude_checks` | — | Check names never dispatched |
| `cluster_checks.ksm_sharding_enabled` | false | Shard `kubernetes_state_core` across runners |
| `clc_runner_enabled` | false | Agent-side: run as a CLC runner |
| `clc_runner_id` / `clc_runner_host` | — | Runner identity and pod IP (downward API) |
| `clc_runner_remote_tagger_enabled` | true | Runners use the DCA's tagger over gRPC |

## Gotchas

- **The warmup lie is load-bearing.** `processNodeStatus` returns `isUpToDate: true` during warmup so node agents keep running cached checks across DCA restarts and leader changes; without it every rolling update of the DCA would bounce every cluster check in the cluster.
- **New checks land on a random node under advanced dispatching** — runner stats are only refreshed at rebalance time, so distribution is intentionally deferred to the next rebalance pass.
- **`unscheduled_check_threshold` is silently clamped** to at least `node_expiration_timeout`; setting it lower has no effect.
- **Endpoints checks are keyed by Kubernetes node name**, cluster checks by `clc_runner_id`/hostname. A CLC runner never receives endpoints checks — they always go to the DaemonSet agent on the pod's node.
- **Double collection is easy.** The DCA registers `kubernetes_apiserver`, KSM, `orchestrator`, and `helm` as its own core checks; if the same integration is also declared with `cluster_check: true` and dispatched to runners, it runs twice.
- **The `RemoveScheduler`-before-`reset()` ordering in [`handler.go`](<<<SRC>>>/pkg/clusteragent/clusterchecks/handler.go) is a deliberate race fix** for KSM sharding — re-ordering it silently drops the KSM check on re-election.
