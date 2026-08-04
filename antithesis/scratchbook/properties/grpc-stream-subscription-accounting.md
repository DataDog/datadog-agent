# grpc-stream-subscription-accounting — gRPC stream subscriptions are never leaked or dropped on reconnect

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 3 discovery agent(s): kubemetadata-stream-subscription-no-drop-on-reconnect, tagger-stream-subscription-accounting-balanced, kubemetadata-stream-reconnect-overlap-witness

## Property

When a node agent's StreamKubeMetadata stream drops and the node reconnects (opening a second, overlapping stream for the same nodeName) before the old stream's deferred Unsubscribe runs, the old handler's cleanup must remove only the channel it created and must never remove the newly-registered channel; every currently-live stream's notify channel remains registered in both the MetaBundleStore.subscribers registry and the namespaceSubscribers registry for as long as that stream runs.


## Invariant / assertion

assert.AlwaysOrUnreachable: for every nodeName, at any observation point the set of channels present in subscribers[nodeName] (and namespaceSubscribers[nodeName]) is exactly the set of channels created by currently-live StreamKubeMetadata handlers for that node — no live handler's channel is absent (no permanent drop), and no returned handler's channel is still present (no leak). Equivalently: len(subscribers[nodeName]) == count of live handlers for nodeName. AlwaysOrUnreachable fits because the overlapping-reconnect path is optional (a run with no stream churn never opens it), but whenever a reconnect overlaps the previous stream's teardown the registry accounting must hold.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Sometimes`: During the run, at least once two distinct live StreamKubeMetadata handlers for the same nodeName are simultaneously registered in the MetaBundleStore (subscribers[nodeName] has length >= 2), proving the hazardous reconnect-overlap window that the no-drop safety invariant guards was genuinely exercised — not merely never opened.


## Antithesis angle

This is the exact bug fixed by #48026 (2b7eb1ece36) and #50670 (8b12b036cea): originally subscribers was map[string]chan struct{}, so a reconnecting node's new Subscribe(nodeName) overwrote the map entry and the OLD handler's `defer Unsubscribe(nodeName)` then deleted the NEW channel — the new stream received the initial full-state snapshot and keepalives but never any diff (permanently dropped subscription, silent). The fix relies on precise interleaving of two goroutines (old handler's deferred cleanup vs new handler's Subscribe) for the same nodeName. Antithesis controls goroutine scheduling and can drive the workload (impersonating one node identity) to drop and immediately reopen the stream repeatedly so the new Subscribe reliably lands before the old defer runs — the window a race-detector unit test can only hit by hand-ordering. No node-termination or clock-skew fault is required: a client-driven stream close (or a workload<->DCA partition that RSTs the old stream) is a sufficient substitute.


## Why it matters

A silently dropped subscription means the node agent's pod-to-service mappings and namespace/Kueue metadata stop updating (only the stale initial snapshot survives) with no error surfaced — tags and metadata quietly go stale cluster-wide for that node. It was a real reported main-branch regression (reported by @gabedos, #48026). Making it a permanent Antithesis regression guard is high value because the fix is entirely interleaving-dependent and invisible to the existing single-goroutine unit tests.


## Mechanism refinement (from open-question investigation)

Scope refinement (not invalidation): the reproducible drop/leak hazard this AlwaysOrUnreachable invariant guards is specific to the nodeName-KEYED kube-metadata registries (subscribers via m.mu and namespaceSubscribers via metadataMutex), which reuse a stable identity across reconnects (client stream.go:488,657). The tagger registry cannot exhibit the same collision because subscriptionID uses a fresh uuid per stream (impl-remote/remote.go:646-648), so the tagger arm reduces to a no-panic / balanced-count guard (regression guard for #40968 map-race), not the drop-on-reconnect bug. Instrument/assert both kube-metadata registries independently (Q8). The tagger arm's activation is additionally gated on the harness running a DCA tagger consumer (Q5, still open).


## Fault dependencies

- workload-driven stream drop+reconnect for the same nodeName (client close / context cancel) — NO node-termination or clock-skew fault required; this is the workload substitute
- optional: asymmetric network partition workload(as node agent)<->DCA to force the old stream to RST while the new one connects (enabled by default)
- concurrency / goroutine interleaving (always on) — the core enabler
- does NOT require leader_election or >=2 replicas (stream serving is not leader-gated), though a >=2-replica run adds reconnect churn via leadership-driven client failover


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Add the Antithesis Go SDK to the root module and instrument pkg/util/kubernetes/apiserver/controllers/store.go and cmd/cluster-agent/api/v1/kubernetes_metadata_stream.go. In StreamKubeMetadata, tag each handler with a unique per-connection token and record (nodeName, token, ch) into a test-only live-handler set on Subscribe and remove it on handler return. Under m.mu / metadataMutex, add assert.AlwaysOrUnreachable that the multiset of channels in subscribers[nodeName] equals the multiset of channels held by currently-live handlers for that node (details: nodeName, lenRegistry, lenLiveHandlers). Also assert.AlwaysOrUnreachable inside Unsubscribe that the channel being removed is one this node's handler actually created (guards against identity confusion). Keep the added tracking map lock-consistent with the registry it mirrors.


## Open questions (post-investigation)

- Is the DCA tagger gRPC stream actually consumed in the target harness topology? Code shows node agents' remote tagger dials the LOCAL core agent (remote.go start()/options.Target), while the DCA tagger server is consumed by in-cluster DCA-targeting clients via cluster_agent.cluster_tagger (e.g. cluster-check runner). Confirm whether the harness runs such a consumer, else the tagger arm of this property is inert. `(partial)`


### Investigation Log

#### Q1: Does the workload hold nodeName stable across reconnects like a real node agent?

Examined comp/core/workloadmeta/collectors/internal/kubemetadata/stream.go:488,657. Found: nodeName is captured once in newDCAStreamClient(nodeName,cfg) (L82,486-488) and reused as sc.nodeName on every StreamKubeMetadata call (L656-657); reconnects reuse the same value. Server keys by req.GetNodeName() (kubernetes_metadata_stream.go:151). Concluded: SUT reuses a fixed node identity across reconnects, so the nodeName-keyed collision reproduces only if the workload likewise holds nodeName stable — a workload-authoring requirement (do not randomize per connection), now confirmed against the real client. Resolved.

#### Q2: Other consumers of Subscribe/Unsubscribe for the same nodeName?

Examined grep of MetaBundleStore.Subscribe/Unsubscribe and GetGlobalMetaBundleStore across pkg/util/kubernetes/apiserver, cmd/cluster-agent, pkg/clusteragent. Found: the ONLY caller is kubernetes_metadata_stream.go:153-154; GetGlobalMetaBundleStore() has a single caller, grpc_kubemetadata.go:19. Other Subscribe/Unsubscribe hits are unrelated types (workloadmeta wlm, RC client, leaderEngine). Concluded: no other consumer can concurrently register/deregister for the same nodeName; accounting is not confounded. Resolved.

#### Q3: Does slices.Delete's in-place shift alias a channel pointer unsafely under m.mu?

Examined store.go: Subscribe (L119-128, m.mu.Lock append), Unsubscribe (L132-150, m.mu.Lock + slices.Delete), notifyLocked (L154-164, requires lock; called only from set()/delete() which hold m.mu.Lock at L99/L109). Get (L58) takes RLock but never touches subscribers. Concluded: every read and write of the subscribers slice header occurs under the m.mu write lock; notify and Unsubscribe are mutually exclusive, no slice header is read outside the lock, so the in-place shift is safe. Resolved.

#### Q4: Can two paths call unsubscribe for the same id (double-decrement)?

Examined tagger subscription_manager.go:91-119 and kube-metadata store.go:132-150. Found: tagger unsubscribe re-checks `found` (L92-96), then delete()+close(sub.ch)+Subscribers.Dec() (L116-118); Notify may force-unsubscribe a full-channel subscriber (L152) racing the handler's deferred Unsubscribe (server.go:125), but the second call hits the not-found guard -> no-op, no double-close, no double-Dec. Kube-metadata store has NO server-initiated force-unsubscribe (notifyLocked only non-blocking sends), so each handler's channel is removed exactly once by its single deferred Unsubscribe; store keeps no per-node gauge. Concluded: no double-decrement/double-close on either path. Resolved.

#### Q5: Is the tagger gRPC stream consumed by node agents (remote_tagger) or only in-cluster?

Examined comp/core/tagger/impl-remote/remote.go (start()/DialContext to options.Target; maxMsgSize from cluster_agent.cluster_tagger.grpc_max_message_size L228) and startTaggerStream L646-648. Found: node agents' remote tagger dials a configured Target (local core agent in the standard node topology); the DCA tagger server (cmd/cluster-agent/api/server.go:131) is consumed by DCA-targeting clients (cluster-check runner via cluster_agent.cluster_tagger). Critically, StreamingID = fmt.Sprintf("%s:%s", flavor, uuid.New()) FRESH per startTaggerStream, so reconnects never collide on subscriptionID. Concluded (partial): the nodeName-style drop-on-reconnect collision CANNOT occur for the tagger path; whether the harness actually runs a DCA tagger consumer is topology-dependent and unresolved from code.

#### Q6: Does the throttler token release path leak a token without leaking a subscriber?

Examined comp/core/tagger/server/syncthrottler.go:55-62 and server.go:114-166. Found: Release is idempotent (guarded by activeRequests `found` check before draining tokensChan and delete). TaggerStreamEntities acquires one token (L115), releases it once initBurst completes (L164) and again via defer (L116) — the second is a guaranteed no-op. Token lifetime is intentionally decoupled from subscription lifetime: the token gates only the initial sync burst, while the subscription persists for the whole stream. Concluded: no token leak (idempotent double-release) and no coupling that could leak a subscriber vs a token. Resolved.

#### Q7: Is len>=2 reachable with a single workload node identity?

Examined store.go:119-128 (Subscribe appends) and the reconnect-overlap mechanism. Found: when a stream drops and the same-nodeName stream reopens before the old handler's deferred Unsubscribe (kubernetes_metadata_stream.go:154) runs, the new Subscribe appends a second channel -> subscribers[nodeName] has length 2 with a single node identity. The multi-process comment (L96-100) is an additional, not required, route. Concluded: len>=2 is reliably reachable with one node identity via drop+reconnect overlap; modeling agent+diagnose separately is unnecessary. Resolved.

#### Q8: Should the witness also fire on the namespaceSubscribers registry independently?

Examined kubernetes_metadata_stream.go:153-157,407-439 vs store.go:119-150. Found: each handler registers in TWO independent registries under DIFFERENT locks — subscribers (m.mu, store.go) and namespaceSubscribers (metadataMutex, stream.go) — each with its own append/identity-delete + deferred Unsubscribe. Because they are separate lock domains, a given schedule can produce an overlap window in one registry but not the other at the same observation instant. Concluded: yes, the witness/assertion should be instrumented on both registries independently rather than assuming they move in lockstep. Resolved.


---

## Source discovery evidence (raw, per contributing agent)


### from `kubemetadata-stream-subscription-no-drop-on-reconnect`

**Confirmed from primary source (commit f2da1471bb):**

- `cmd/cluster-agent/api/v1/kubernetes_metadata_stream.go:150-157` — each `StreamKubeMetadata` handler does `podServicesNotifyCh := srv.store.Subscribe(nodeName); defer srv.store.Unsubscribe(nodeName, podServicesNotifyCh)` and `namespacesNotifyCh := srv.subscribeToNamespaceEvents(nodeName); defer srv.unsubscribeFromNamespaceEvents(nodeName, namespacesNotifyCh)`. The subscription is keyed by `nodeName`, which is stable across a node's reconnects (`req.GetNodeName()`, L151).
- `pkg/util/kubernetes/apiserver/controllers/store.go:49` — `subscribers map[string][]chan struct{}` (slice per node). `Subscribe` (L119-128) appends a fresh buffered(1) channel; `Unsubscribe(nodeName, ch)` (L132-150) removes by **pointer identity** via `slices.Delete` and deletes the map key only when the slice empties.
- `kubernetes_metadata_stream.go:407-439` — `subscribeToNamespaceEvents`/`unsubscribeFromNamespaceEvents` mirror this exactly (append + identity delete) under `metadataMutex`.
- `notifyLocked` (store.go:154-164) and `notifyNamespaceSubscribers` (stream.go:393-405) fan out to **all** channels in the slice with a non-blocking send.

**Fix history proving the mechanism (this is a regression guard, not a live defect):**

- `git show 2b7eb1ece36` (#48026, "Fix stream unsubscribe race when node reconnects"): commit message states *"a reconnecting node's new subscription channel could be removed by the old connection's deferred cleanup, causing the new connection to never receive notifications."* Diff changed `Unsubscribe(nodeName)` → `Unsubscribe(nodeName, ch)` guarded `if m.subscribers[nodeName] == ch { delete(...) }`.
- `git show 8b12b036cea` (#50670, "Allow multiple subscribers per node"): changed the single-channel map to a slice so concurrent overlapping streams for one node can coexist.

**Not leader-gated:** `cmd/cluster-agent/api/server.go:128-133` registers `serverSecure{taggerServer, kubeMetadataServer}` unconditionally at API-server start; `grpc_kubemetadata.go:18-22` starts the streamer with no `IsLeader` check. Both leader and followers serve these streams, so the invariant is per-replica and independent of leadership.

**Instrumentation target:** the invariant is on DCA-internal registry state not observable from the workload, so it needs SUT-side SDK assertions (net-new; `existing-assertions.md` = zero).


### from `tagger-stream-subscription-accounting-balanced`

**Grounding (commit f2da1471bb):**

- `comp/core/tagger/server/server.go:114-160` — `TaggerStreamEntities` calls `s.taggerComponent.Subscribe(subscriptionID, filter)` then `defer subscription.Unsubscribe()`; subscriptionID = `"streaming-client-" + streamingID` (L104-108), streamingID from the client or a server-generated uuid.
- `comp/core/tagger/impl-remote/remote.go:646-650` — the remote tagger client sets `StreamingID: fmt.Sprintf("%s:%s", flavor.GetFlavor(), uuid.New().String())` **fresh on every `startTaggerStream`**, so reconnects do not collide on subscriptionID (contrast with the nodeName-keyed kube-metadata path).
- `comp/core/tagger/subscriber/subscription_manager.go:52-87` — `Subscribe` holds `sm.Lock()` across the duplicate-id check + insert (this ordering is the #40968 fix); `sm.telemetryStore.Subscribers.Inc()` at L74.
- `subscription_manager.go:91-119` — `unsubscribe` deletes the id, `close(sub.ch)`, `Subscribers.Dec()`. `Notify` (L130-164) may call `sm.unsubscribe(subscriber.id)` at L152 when a subscriber's channel is full — a server-initiated cancellation that races the handler's own deferred `Unsubscribe`.

**Fix history:** `git show 8075e2291fc` (#40968, "Fix race condition in tagger Subscribe method") — commit message: *"The datadog-cluster-agent service was experiencing fatal 'concurrent map read and map write' errors in the tagger subscriber component"*; fix moved `sm.Lock()` above the `sm.subscribers[id]` read. This is a confirmed real DCA crash, making the accounting/no-panic invariant a genuine regression guard.

Baseline SDK instrumentation is zero (`existing-assertions.md`).


### from `kubemetadata-stream-reconnect-overlap-witness`

**Grounding (commit f2da1471bb):**

- `pkg/util/kubernetes/apiserver/controllers/store.go:119-128` — `Subscribe` appends to `subscribers[nodeName]`; a slice length >= 2 is only reachable when two handlers for the same node overlap in time (#50670's whole purpose, `git show 8b12b036cea`).
- `store.go:132-150` — `Unsubscribe` removes by identity via `slices.Delete`; removing a non-head element is direct evidence that a later Subscribe interleaved before an earlier Unsubscribe (the exact ordering the #48026 fix, `2b7eb1ece36`, guards).
- `cmd/cluster-agent/api/v1/kubernetes_metadata_stream.go:96-100` documents that multiple concurrent subscribers per node are expected ("the running agent plus 'agent diagnose', 'agent check', etc."), so overlap is a real, intended condition — the witness confirms the test reaches it.

This witness observes only DCA-internal registry length, so it is SUT-side SDK instrumentation (net-new; baseline zero per `existing-assertions.md`).
