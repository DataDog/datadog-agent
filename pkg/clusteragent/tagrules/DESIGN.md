# Tag Rules: Custom Tags for Kubernetes Metrics — Design

Status: POC (innovation week) · Cluster Agent + Node Agent · `agent.datadoghq.com/v1alpha1`

## 1. Problem

Customers need to attach custom tags to their Kubernetes metrics, scoped to certain objects, where the tag value depends on cluster state that is not a pod label. Three motivating cases:

1. **Leader pods.** A customer wants `container.memory.*` for a controller's pods, but only for the current leader, identified by comparing the leader-election Lease holder with the pod name. No label carries this; it changes over time.
2. **Cordoned nodes.** A customer wants `system.cpu.*` and `kubernetes.cpu.usage` on a node to carry `is_schedulable:false` while the node is cordoned. The state lives in the Node object and flips without any label change.
3. **Tag precedence.** A customer maps an `owner` label to `owning_team` on both pods and namespaces, and wants the pod value to win, falling back to the namespace value. Existing label-as-tags features emit both values and leave deduplication to every query.

None of this is expressible today without writing a custom check with custom metrics.

## 2. Goal

Provide a generic mechanism for customers to apply custom tags to metrics, scoped to selected Kubernetes objects, evaluated against cluster state, without custom metrics pipelines and without agent configuration on every node.

### Non-goals (v1)

- **Metric-name scoping.** A rule's tags land on *all* telemetry sourced from the matching object. There is no `applies_to: container.memory.*` filter — the annotation pickup path the design builds on is not metric-aware.
- **Open-ended string values.** v1 allows only booleans and values from a declared, closed set. This is the cardinality contract (see §7).
- **Arbitrary entities.** Pods and nodes only. Pods cover most workloads; nodes were required by motivating case 2.

## 3. Architecture at a glance

```
             ┌────────────────────────────────────────────────┐
             │                 Cluster Agent                  │
             │                                                │
  TagRule ─▶ │  validating webhook ──▶ tag-rule controller ──┼──▶ writes tags as
  (CR)       │  (fail-closed)          (leader-elected)       │      annotations
             └────────────────────────────────────────────────┘
                                        │
              pod: ad.datadoghq.com/<container>.tags (JSON)    │ node: tags.datadoghq.com/tag.<key>
                                        │                       │
             ┌─────────────────────────▼───────────────────────▼──────┐
             │ Node Agent: tagger reads pod annotations               │
             │             host-tags flow reads node prefix annotations│
             └──────────────────────────────────────────────────────────┘
                                        │
                                 metrics with custom tags
```

Four cooperating pieces:

1. **A cluster-scoped CRD, `TagRule`** (`agent.datadoghq.com/v1alpha1`), one rule per CR. The API group is dedicated to the Agent to avoid sharing a group with other Datadog operators.
2. **A validating admission webhook** in the Cluster Agent, on its own HTTPS server, enforcing the invariants the schema cannot express — most importantly global tag-key ownership and expression well-formedness. It fails closed: if it is down or broken, no rule is admitted.
3. **A controller** in the Cluster Agent, running only on the elected leader. It watches rules, pods, nodes and rule-declared source objects; evaluates rules; and writes tags as **annotations** on matching entities. The node agents' existing tagger picks pod annotations up unconditionally, so pod rules reach metrics with no agent-side change.
4. **A small agent-side change (A2)** for node rules: node-level host metrics (`system.cpu.*`) never went through the tagger, so the host-tags flow now unconditionally reads the reserved annotation prefix on the node's object and stamps those tags on metrics continuously, on a 15-second refresh, without any customer configuration.

Rules are installed like any other CRD: applied by the chart (or kubectl for the POC) alongside the RBAC and the webhook configuration.

## 4. The rule contract

A rule says: **for every object of this kind matching this selector, a tag with this key carries this computed value.**

```yaml
apiVersion: agent.datadoghq.com/v1alpha1
kind: TagRule
metadata:
  name: is-leader-my-controller
spec:
  entity: pod
  selector:
    namespace: my-namespace
    matchLabels:
      app.kubernetes.io/name: my-controller
  tag: is_leader
  source:
    kind: Lease
    namespace: entity.metadata.namespace
    name: entity.metadata.labels['app'] + '-leader-election'
  value:
    type: bool
    bool:
      expression: source.spec.holderIdentity == entity.metadata.name
  flipHoldSeconds: 15
```

- **entity** — `pod` or `node`. Selectors use the standard label-selector grammar plus an optional namespace scope (pods only; nodes are cluster-scoped and a namespace selector is rejected). An empty selector matches everything of the kind.
- **tag** — the tag key. It is **globally unique across the cluster**: exactly one rule owns a given tag key, enforced by the webhook and resolved deterministically by the controller if bypassed (lexicographically smallest CR name wins; the loser is evicted, flagged in its status, and its key swept). The key is immutable after creation.
- **source** — an optional second object exposed to value expressions as `source`, resolved per entity. Supported v1 kinds: Pod, Node, Namespace, Lease, Deployment, Service. Its name and namespace are themselves string expressions evaluated against the entity, defaulting to the entity's own name/namespace. Omitting the source (or naming the entity's own kind without a name) evaluates the entity against itself — that is how the cordon case works. A missing source object is treated like an evaluation failure and follows the rule's error policy.
- **value** — one of:
  - **bool** — an expression evaluating to true/false. Source-object comparison is a first-class use (the Lease example above).
  - **string set** — an expression evaluating to a string, **constrained to a declared, non-empty set of allowed values**. When the expression fails or produces a value outside the set, the rule's error policy applies: `drop` omits the tag, or `default` writes a declared fallback (which must itself be a member of the set).
- **flipHoldSeconds** — the minimum time a value must hold before a flip is written; floor 15, matching the fleet cadence.

Expressions are [CEL](https://github.com/google/cel-spec), evaluated over `entity` and `source` as unstructured object maps. Identifier-safe fields use dot access (`entity.spec.unschedulable`); keys with dots need brackets (`entity.metadata.labels['app.kubernetes.io/name']`). Presence tests use `in` (`'owner' in entity.metadata.labels`); objects without labels or annotations are normalized so presence tests behave. The value is always computed declaratively from current state — the controller recomputes the full desired tag set on every reconcile and writes at most one patch per entity per pass.

## 5. Mutation semantics

The controller writes into shared annotation surface, so ownership must be explicit:

- **Pods**: tags merge into the per-container annotation `ad.datadoghq.com/<container>.tags` — a JSON map the tagger already reads unconditionally and dynamically. User-set keys inside that JSON are never removed. A container whose annotation exists but is not valid JSON is left untouched (the controller never risks destroying user data for a tag). Ownership is recorded on the pod in a ledger annotation, `datadoghq.com/managed-tag-keys`, which is what makes stripping stale keys and surviving restarts correct.
- **Nodes**: tags are written as individual annotations under the reserved prefix `tags.datadoghq.com/tag.<key>`. The prefix is itself the ownership marker.
- **Drift**: the controller wins. A direct user edit of a controller-owned value is restored on the next reconcile (15s at the worst).
- **Deletion**: every rule carries the finalizer `datadoghq.com/tag-rule-cleanup`. On deletion, the controller sweeps all entities that matched the rule's last-observed selector — recomputing them without the rule strips the owned key and updates the ledger — and only then releases the finalizer, so the CR cannot disappear before its tags do.
- **Flapping**: a value that flips again inside the hold keeps the previously written value. The hold governs disappearances too, so a tag never drops out faster than the hold allows. State is in-memory; a restart re-initializes from what is written on entities.

One semantic consequence, inherent to any annotation-based scheme: the tag value is time-varying, so the *series identity* is time-varying — a pod that holds and loses leadership produces two distinct series over its lifetime, and queries must expect the tag to appear on one side of a flip only. Historical points keep the value they were submitted with; nothing is retroactively re-tagged.

## 6. Runtime behavior

- **Leadership**: the controller reconciles only on the elected Cluster Agent. Followers keep informers warm but defer all work; on gaining leadership, full rules are picked up within one resync interval.
- **Cadences**: status/resync loops run on 15s defaults (configurable); the flip-hold floor is not a knob — it is a fleet-wide invariant.
- **Sources**: watched via informers started on demand, only for the kinds rules actually reference. A source informer that has not yet synced produces *no decision* — the entity keeps whatever it currently carries rather than being stamped with a fallback value. (This mattered: without it, every Cluster Agent restart would briefly stamp `onError` defaults fleet-wide before flipping them back.)
- **Status**: each rule reports state (active/error), matched/tagged entity counts, a per-value occurrence histogram for string-set rules, and dropped-write counts — the runtime half of the cardinality guard, visible to the customer.

## 7. The cardinality model

The design's primary risk is high-cardinality tags. The guard is layered:

1. **By value kind.** A bool has two values. A string set is bounded by its declared set — the set is mandatory, and the controller enforces membership at runtime.
2. **By the webhook.** Structural and cross-field checks live in the CRD schema itself (CEL validation rules); the webhook adds what the schema cannot see: expression compile/type/cost checks, duplicate set entries, and global tag-key ownership. Fail-closed.
3. **By the controller.** Values outside the declared set never reach an annotation; the error policy decides between omission and the declared fallback.

An open-ended escape hatch (a numeric cardinality budget with runtime quarantine) was deliberately deferred: it reintroduces exactly the risk this feature exists to bound.

## 8. The agent-side change (A2)

Node-emitted host metrics never went through the tagger, and host tags were only attached to metrics during a short startup window. The change, scoped to the reserved prefix only:

- The node's annotations with the reserved prefix map to tags with no configuration, refreshed by a single-object read against the API server every 15 seconds — small enough per agent; a shared watch is the documented follow-up.
- Those tags are attached to host metrics for the process lifetime, on their own refresh; a failed refresh keeps the last known values (a transport error must not drop tags from metrics).
- All other host tags keep their existing semantics exactly.
- They also enter the periodic host-metadata submission, so customers without the continuous path still get the tag (coarse-grained).

End-to-end, a flip is visible on host metrics within roughly 30 seconds worst case (15s controller hold + 15s refresh).

## 9. Failure modes

| Failure | Behavior |
|---|---|
| Webhook down | No rule changes admitted (fail-closed) |
| CRD missing | Controller waits with backoff; the Cluster Agent is unaffected |
| Cluster Agent loses leadership | Controller defers all work; new leader resumes within one interval |
| Source object missing | Error policy per rule: tag dropped or declared fallback |
| Source informer not synced | No decision; entity keeps current tags |
| Evaluation error | Logged per entity; dropped writes counted in rule status |
| Invalid rule admitted despite everything | Controller re-validates; refuses to write; rule status reports the reason |
| Node agent cannot read node from API server | Node-rule tags silently degrade (documented RBAC requirement) |
| Two rules claim one tag key | Webhook rejects; controller resolves deterministically if bypassed |

## 10. Installation and configuration

- **CRD, RBAC, ValidatingWebhookConfiguration** — applied by the chart when the feature is enabled (standalone install manifests exist for the POC). The webhook runs on its own port (default 9443) and its configuration uses `failurePolicy: Fail`.
- **Configuration keys** (Cluster Agent): `cluster_agent.tag_rules.enabled`, `.status_interval`, `.resync_interval` (defaults 15s), `.webhook.enabled`, `.webhook.port`. Defaults are all off.
- **RBAC** (Cluster Agent service account): full TagRule access; read + patch on pods and nodes; read on namespaces and leases.
- **POC TLS note**: without a configured certificate pair, the webhook serves a self-signed certificate and logs the base64 CA bundle to paste into the webhook configuration's `caBundle`. Production requires chart-managed certificates.

## 11. Verification summary

- Controller: unit suites for evaluation, selection, annotation merge (user-key preservation, stale stripping, unparseable-JSON safety, ledger updates), flip-hold; fake-cluster integration suites covering the three motivating examples end-to-end (including leadership flips, source-object changes, cordon/uncordon, drift restoration, deletion cleanup, conflict resolution, status reporting).
- Webhook: end-to-end request/response tests over the real handler, including every denial path and the fail-closed behaviors.
- Host-tags: build-variant stubs, refresh semantics, race-clean.
- Full Cluster Agent binary builds with the canonical build-tag set; all touched packages pass vet, gofmt, and tests.

## 12. Future work

- Metric-name scoping (`applies_to`) — needs either tagger-side metric awareness or a backend ingestion-time evaluator; the rule format reserves the concept.
- A dedicated informer/watch for node-prefix annotations instead of the 15s read, and a singleton fetcher shared across the agent's tag consumers.
- Extending the Cluster Agent's node-annotation endpoint to serve the reserved prefix, removing the node agents' direct API server dependency.
- The orchestrator-based backend evaluation path (ingestion-time tags from Datadog's cluster view) as an alternative deployment of the same rule format.
- Runtime cardinality quarantine for a bounded open-ended value mode, if ever needed.

## Appendix: the three motivating examples as rules

```yaml
# 1. Leader pods (bool + source comparison)
entity: pod
selector: {namespace: my-namespace, matchLabels: {app: my-controller}}
tag: is_leader
source: {kind: Lease, name: entity.metadata.labels['app'] + '-leader-election',
         namespace: entity.metadata.namespace}
value: {type: bool, expression: source.spec.holderIdentity == entity.metadata.name}

# 2. Cordoned nodes (bool, self source)
entity: node
selector: {}
tag: is_schedulable
value: {type: bool, expression: '!has(entity.spec.unschedulable) || !entity.spec.unschedulable'}

# 3. Tag precedence (string set with fallback)
entity: pod
selector: {}
tag: owning_team
source: {kind: Namespace, name: entity.metadata.namespace}
value:
  type: string_set
  stringSet:
    values: [frontend, backend, data, infra, unknown]
    expression: "('owner' in entity.metadata.labels)
                 ? entity.metadata.labels['owner']
                 : source.metadata.labels['owner']"
    onError: default
    default: unknown
```

Exactly one `owning_team` tag is emitted — precedence is resolved at reconcile time, so nothing downstream needs to deduplicate.
