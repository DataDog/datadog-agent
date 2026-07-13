# DatadogInstrumentation (DDI) Controller & Platform

This package implements the **DatadogInstrumentation (DDI) platform**: a shared
Cluster Agent controller that watches `DatadogInstrumentation` custom resources and
dispatches per-product events to pluggable **handlers**. It is a *platform* meant to
be extended by future teams (Autodiscovery today; SSI, AppSec, and others later),
not a single-purpose controller.

- Package: `github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation`
- Build tag: `//go:build kubeapiserver` (all files here and in `handlers/`)
- CRD type: `github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1.DatadogInstrumentation`
- GVR: `datadoghq.com/v1alpha1`, resource `datadoginstrumentations` (namespace-scoped)

## Why this exists

Customers configure workload-specific Datadog features (Autodiscovery checks, APM
SSI, AppSec) through pod annotations, container labels, or static Agent config.
Those approaches require workload/Agent rollouts, fail silently on bad input, and
each product reinvents its own targeting and validation. The DDI CRD replaces this
with one declarative, namespace-scoped resource per workload that:

- targets a single workload via `spec.targetRef` (1:1, bounded blast radius),
- carries product-specific config under `spec.config.<section>`,
- validates synchronously at admission (immediate feedback, no silent failures),
- reports per-product readiness back as Kubernetes status conditions.

Design references (read before non-trivial changes):
- RFC: [DatadogInstrumentation CRD for Workload-Scoped Product Enablement](https://datadoghq.atlassian.net/wiki/spaces/CONTP/pages/6564659495)
- [Autodiscovery Handler for DatadogInstrumentation](https://datadoghq.atlassian.net/wiki/spaces/CONTP/pages/6579552410)
- Epic: [CONTP-1453](https://datadoghq.atlassian.net/browse/CONTP-1453)

## The CRD

Defined in the datadog-operator API module, **not** in this repo. Changing the
schema means bumping the `github.com/DataDog/datadog-operator/api` dependency; you
cannot edit the type here.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogInstrumentation
metadata:
  name: example
  namespace: default
spec:
  targetRef:                 # shared: one workload per CR, immutable after create
    apiVersion: apps/v1
    kind: Deployment
    name: my-service
  config:                    # each key is one product "section", owned by a handler
    checks: [...]
    logs: [...]
status:
  conditions:                # one condition per handler that acted
    - type: ChecksReady
      status: "True"
```

Shared fields (`spec.targetRef`, `status.conditions`) are the platform's concern.
Everything under `spec.config` is owned and defined by individual handlers.

## Package layout

### `pkg/clusteragent/instrumentation/` (platform core, product-agnostic)

| File | Responsibility |
|------|----------------|
| `types.go` | `Handler` interface, `EventType`, `HandlerStatus`, `ValidationError`, `DatadogInstrumentationGVR`. The platform contract. |
| `controller.go` | `Controller`: informer wiring, workqueue, reconcile loop, `lastSeen` cache, leader-gated status writes. |
| `events.go` | `classifySectionEvent` (old/new to Create/Update/Delete/skip) and `DatadogInstrumentationFromObject` (typed/unstructured/tombstone to typed). |
| `status.go` | `updateStatusConditions`: leader-only, conflict-retried write of one condition per `HandlerStatus`. |
| `conversion.go` | Unstructured/typed conversion helpers (the informer is dynamic/unstructured). |

The core imports nothing product-specific: only the CRD type, client-go, and
`pkg/util/log`. All product knowledge lives in handlers.

### `pkg/clusteragent/instrumentation/handlers/` (product implementations)

`registry.go` defines `Deps` (shared dependencies) and `DefaultHandlers(deps)`, the
list of handlers the controller and webhook run. The current handlers are
`ChecksHandler` and `LogsHandler`. Their internals are handler-specific and not the
platform's concern; read those files directly if you need their behavior.

## How reconciliation works

1. Informer events enqueue the CR key. Updates where `metadata.generation` is
   unchanged are skipped (status-only updates, resyncs).
2. `reconcile(key)` loads the current CR from the lister and the previously
   reconciled CR from the in-memory `lastSeen` map.
3. For each handler, `classifySectionEvent` compares `handler.HasSection(old)` vs
   `handler.HasSection(new)`:

   | HasSection(old) | HasSection(new) | Event |
   |---|---|---|
   | false | true | `EventCreate` |
   | true | true | `EventUpdate` |
   | true | false | `EventDelete` |
   | false | false | skip |

   One CR update can produce different events for different handlers. On a delete
   event the handler receives the **old** CR (the new one is gone).
4. `handler.Handle(ctx, event, cr)` returns a `HandlerStatus`. A handler error
   aborts the reconcile and requeues (rate-limited, up to `maxRetries=3`).
5. `setLastSeen` records the new state for next round.
6. Status is written only if `isLeader()`: `updateStatusConditions` re-fetches the
   CR and sets one condition per non-empty `HandlerStatus.Type` with conflict retry.

### Leadership semantics (important)

**Handlers run on every replica** (leader and followers), because some handlers must
act regardless of leadership. **Only the leader writes CR status.**

- Guard cluster-wide side effects (creating k8s objects, calling external APIs) with
  `deps.IsLeader()` yourself; the platform will not.
- Do **not** gate purely local per-replica work on leadership.
- Known gap: a handler error on a follower is not reflected in status if the leader
  succeeded (status is leader-only).

### Idempotency

`Handle` must be idempotent. The controller may deliver duplicate events for the
same change (resyncs, requeues).

## Validation webhook

`pkg/clusteragent/admission/validate/datadoginstrumentation/webhook.go` is a
validating admission webhook that shares the same handler list as the controller and
validates in ordered stages:

1. `targetRef` immutability (Update only).
2. Unique `targetRef` per namespace.
3. Target compatibility: for each handler where `HasSection` is true, reject if
   `SupportsTarget(targetRef)` is false.
4. Product validation: `handler.Validate(cr)`; any `ValidationError` rejects.

Because the webhook and controller share the handler list, your handler's
`Validate` and `SupportsTarget` are the single source of truth for both admission
and reconcile.

## Extending the platform: building a new handler

### 1. Add the CRD section (operator API)
Add `spec.config.<yours>` to the type in the datadog-operator API module and bump
`github.com/DataDog/datadog-operator/api` in `go.mod` (`dda inv tidy`). The schema
cannot be defined in this repo.

### 2. Implement the `Handler` interface
Create `handlers/<yours>.go` (`//go:build kubeapiserver`) and implement all methods:

```go
type Handler interface {
    Name() string                                                  // unique, stable
    HasSection(*datadoghq.DatadogInstrumentation) bool             // does the CR carry my section?
    SupportsTarget(autoscalingv2.CrossVersionObjectReference) bool // target kinds I accept
    Handle(context.Context, EventType, *datadoghq.DatadogInstrumentation) (HandlerStatus, error)
    Validate(*datadoghq.DatadogInstrumentation) []ValidationError  // synchronous admission checks
}
```

Contract:
- `HasSection` must be a pure, cheap predicate on the CR (e.g.
  `len(cr.Spec.Config.Checks) > 0`). It drives event classification in *both* the
  controller and the webhook.
- `Handle` must be idempotent and handle all three `EventType`s, including
  `EventDelete` (clean up whatever the earlier events created). Return a
  `HandlerStatus` with a stable `Type` that becomes the CR status condition. Return
  an `error` only for transient failures worth a rate-limited requeue; prefer a
  false status for product-level failures you do not want retried.
- Guard leader-only side effects with `deps.IsLeader()`.
- `Validate` returns `ValidationError`s keyed to the offending field for good
  `kubectl` feedback.

How a handler applies its config (delivering to Node Agents, mutating pods, etc.) is
entirely handler-specific. The platform only dispatches events and records status.

### 3. Register the handler
Add any shared dependency to `handlers.Deps` (so it can be shared with the webhook
and other surfaces), then add your constructor to `DefaultHandlers` in
`registry.go`. Both the controller and the webhook receive this slice, so
registering once covers reconcile and admission.

## Wiring / entry points

| Concern | Location |
|---------|----------|
| Controller start (waits for CRD, then `NewController(...).Run`) | `pkg/util/kubernetes/apiserver/controllers/instrumentation_controller.go` |
| Handler slice on `ControllerContext` | `pkg/util/kubernetes/apiserver/controllers/controllers.go` |
| Handler construction and registration | `cmd/cluster-agent/subcommands/start/command.go` (`setupInstrumentationCRDHandler`) |
| Validating webhook | `pkg/clusteragent/admission/validate/datadoginstrumentation/webhook.go` |

## Configuration & gating

- `admission_controller.enabled` **and** `admission_controller.validation.enabled`
  must be true; the controller start path bails otherwise.
- `instrumentation_crd_controller.enabled` (default `false`,
  `pkg/config/setup/common_settings.go`) gates the webhook and related node-agent
  wiring.
- The controller starts only after the DDI CRD exists in the cluster
  (`waitForInstrumentationCRD` retries with backoff; a missing CRD is not fatal).

## Testing

Run `dda inv test --targets=./pkg/clusteragent/instrumentation/...` (the
`kubeapiserver` tag is applied automatically). Core tests use a `dynamic/fake`
client. When adding a handler, cover event classification for your section
(Create/Update/Delete/skip), idempotent re-apply, delete cleanup, `Validate`
rejections, and `SupportsTarget` for each kind you accept and reject.

## Gotchas

- Handlers run on all replicas; status is leader-only. Gate cluster-wide side
  effects on `IsLeader()`; do not gate local per-replica work.
- `Handle` must be idempotent; duplicate events are expected.
- `HasSection` drives event dispatch in both the controller and the webhook. A wrong
  predicate silently drops events.
- `targetRef` is immutable and unique per namespace, enforced only at the webhook.
- The CRD schema lives in the operator module, not here.
- The informer is dynamic/unstructured; always convert via
  `DatadogInstrumentationFromObject` and handle the tombstone case (the helpers do).
