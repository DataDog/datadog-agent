# informer-fresh-or-staleness-surfaced — DCA does not silently serve authoritative data from a frozen informer

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 2 discovery agent(s): informer-cache-fresh-or-staleness-surfaced, informer-freeze-window-witnessed

## Property

When a DCA replica is partitioned from the kube-apiserver such that a watch stream is silently dropped (no RST/FIN), the DCA must not keep serving informer-backed data (admission cert, kube-service/endpoint metadata, CRD state) as authoritative and current while continuing to report Ready/healthy. Either the lister-served value converges to apiserver ground truth within a bounded staleness budget, or the DCA surfaces the staleness (explicit error, staleness metric, or readiness=unready).


## Invariant / assertion

assert.AlwaysOrUnreachable(informer_backed_value_is_fresh_or_staleness_is_surfaced): at every point where an informer-backed lister value is used on a decision path AND the DCA reports healthy, the backing informer has observed a successful watch event or resync within a bounded window B (or the value equals current apiserver ground truth). AlwaysOrUnreachable fits because the informer paths are optional (admission/metadata/CRD may be disabled) but whenever exercised the freshness-or-surfaced invariant must hold. EXPECTED TO FAIL today: there is no post-startup freshness check (HasSynced stays true forever after the initial WaitForCacheSync), no staleness metric, and readiness is not tied to informer liveness, so under a watch-drop partition the DCA serves stale data silently — that failing trace is the deliverable.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: The hazardous precondition for the root-cause property is actually scheduled: during a silent watch-drop partition, a DCA replica's informer-backed lister returns a value that differs from the current apiserver ground truth for the same object (i.e., the cache genuinely froze and the staleness window opened).


## Antithesis angle

kubernetes_apiserver_informer_client_timeout defaults to 0 (common_settings.go:2039, core_schema.yaml:9180). That flows to defaultInformerTimeout (apiserver.go:183), is handed to every informer client (apiserver.go:402-432), and becomes rest.Config.Timeout=0 (GetClientConfig, apiserver.go:266); the CustomRoundTripper wrap only logs timeouts, it adds no deadline (roundtrip.go:37-47). So informer watch requests have no client-side timeout. Inject an asymmetric/blackhole partition (drop packets, no RST) between one DCA replica and the apiserver, then MUTATE the watched object on the apiserver from the workload: rotate the admission cert Secret (admission_controller.certificate.secret_name), change the DCA Service EndpointSlice, or update kube-service endpoints. Assert the replica's lister-served value converges to the new ground truth within B, or the replica flips unready / surfaces staleness. This is a state only Antithesis reaches: a silent watch drop concurrent with a real data change — a fake clientset cannot reproduce it.


## Why it matters

This is the ROOT CAUSE behind several symptom properties (stale admission cert -> opaque TLS failures or wrong cert after rotation; stale endpoint/service metadata; stale CRD-derived state). A frozen informer surfaces no error and keeps the pod Ready, so operators see a healthy DCA silently serving wrong data cluster-wide with no alert. Because HasSynced latches true after the one-time startup sync, there is no built-in freshness signal anywhere.


## Mechanism refinement (from open-question investigation)

REFINEMENT (does not invalidate): The Open-Q1/Q5 and Antithesis-angle premise that a 0 client-side informer timeout leaves the watch hanging indefinitely is incomplete. client-go's default transport enables an HTTP/2 connection health-check ping (ReadIdleTimeout=30s, PingTimeout=15s; k8s.io/apimachinery util/net/http.go:187-188, wired via client-go transport/cache.go:134), and the agent does not disable it. So a silent blackhole is detected in ~30-45s and forces a relist; the stale window is bounded by the partition duration (self-heals on partition end), not unbounded. The core defect still holds and the property stands: HasSynced latches true after startup, there is no staleness metric, and readiness only drains a liveness ping (admission server.go:176) with no informer-freshness gate — so during the partition the DCA silently serves stale lister values (cert per TLS handshake, endpoint/metadata to node agents) while reporting Ready. Recommend the assertion's staleness budget B be set to bound the DURING-PARTITION window (e.g. detection ~45s plus partition length), and drop any 'freeze is indefinite' framing.


## Fault dependencies

- network partition DCA-replica <-> kube-apiserver, silent/blackhole drop (no RST/FIN) so the watch stream hangs — enabled by default on most tenants
- workload-driven mutation of the watched object during the partition (rotate admission Secret / change DCA EndpointSlice / update kube-service endpoints) — the topology already gives the workload apiserver ownership of Service/EndpointSlice objects
- no node termination or clock skew required
- requires whichever informer path is enabled: admission_controller.enabled, or kube-service metadata, or a CRD controller


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

Net-new (zero existing SDK usage per existing-assertions.md). Add the Antithesis Go SDK to the root module and instrument at each lister read on a decision path: (1) record the informer's last-observed watch-event/resync timestamp (or LastSyncResourceVersion) alongside the served value; (2) at the read, emit assert.AlwaysOrUnreachable(fresh_or_surfaced) comparing (now - lastSync) < B OR staleness surfaced. Simplest concrete probe: instrument GetCertificateFromLister (admission) and the endpoint/metadata lister read. Pair with the witness property below so a green result is not vacuous. Because the assertion is expected to fail, the deliverable is the reproducing trace: stale value served while the pod is Ready.


## Open questions (post-investigation)

- Which staleness budget B is defensible per surface? No B is defined anywhere in code (HasSynced latches, no staleness metric). Two natural bounds exist: (a) ~45s = HTTP/2 dead-connection detection window (relist trigger); (b) the per-surface correctness window (cert rotation vs endpoint churn). Choosing the authoritative B is an intended-behavior call. `(needs human input)`
- Which surface (admission Secret / EndpointSlice / kube-endpoints / CRD) most reliably opens the freeze window under the harness partition primitive? From code the admission cert is the most deterministic decision-path read (read from secretsLister on every TLS handshake, server.go:137-140); metadata is served from a reconciled store fed by informer events. Ranking requires empirical harness measurement. `(partial)`


### Investigation Log

#### Q1: Does client-go HTTP/2 ReadIdleTimeout ping detect the blackholed connection and relist, or block indefinitely?

Examined k8s.io/client-go@v0.35.5/transport/cache.go:134 (builds transport via utilnet.SetTransportDefaults) and k8s.io/apimachinery@v0.35.5/pkg/util/net/http.go:131-190. Found: SetTransportDefaults calls configureHTTP2Transport, which unconditionally sets t2.ReadIdleTimeout=30s and t2.PingTimeout=15s unless DISABLE_HTTP2 or HTTP2_READ_IDLE_TIMEOUT_SECONDS=0 env vars are set. Grep confirms the agent sets neither (no DISABLE_HTTP2 anywhere in repo). The rest.Config path (no custom Transport; only clientConfig.Wrap of the RoundTripper, apiserver.go:274) uses this default transport. Conclusion: the HTTP/2 health-check ping IS enabled by default. Under a silent blackhole, the ping fails and the dead connection is torn down after ~30-45s, forcing a watch relist. The 0 client-side timeout (kubernetes_apiserver_informer_client_timeout=0) does NOT make the freeze indefinite — transport-layer ping bounds it. RESOLVED.

#### Q3: Does any readiness/health check flip unready under a frozen informer?

Examined cmd/cluster-agent/admission/server.go:91,176-178 and pkg/status/health/global.go. Found: the admission webhook registers health.RegisterReadiness('admission-controller-webhook') but the Run loop only drains s.healthHandle.C ('// Drain the health check channel to stay healthy'). The health package is a pure goroutine-liveness ping (30s timeout); it has zero coupling to informer watch liveness or LastSyncResourceVersion. No readiness gate anywhere ties to informer freshness. Conclusion: readiness is fully decoupled from informer liveness; a frozen informer keeps the pod Ready. RESOLVED (confirms property premise).

#### Q4: Are endpoints/metadata served to node agents read from the informer lister or re-fetched directly?

Examined pkg/util/kubernetes/apiserver/controllers/metadata_controller.go:98-110,296 and cmd/cluster-agent/admission/server.go:137-140. Found: metadata controller reads m.endpointSliceLister / m.endpointsLister.Endpoints(ns).Get(name) (line 296) — informer-backed lister; results reconciled into globalMetaBundleStore and served to node agents. Admission cert read from s.secretsLister on every TLS handshake. Both are informer-lister-backed and WILL freeze. Contrast: GetLeaderIP uses direct Endpoints().Get/EndpointSlices().List (per discovery evidence, leaderelection.go) — not informer, would not freeze. Conclusion: the node-agent metadata/endpoint path and the cert path are lister-backed. RESOLVED.

#### Q5: How long does the divergence persist (partition-bounded vs indefinite)?

Derived from Q1 plus apiserver.go:184 / common_settings.go:565. Found: HTTP/2 ping tears down the connection ~30-45s after silence and triggers a relist; the relist fails/retries during the partition, so the cache stays stale for the partition duration, then converges once the partition heals. kubernetes_informers_resync_period default 300s does NOT help — client-go resync re-delivers cached objects to handlers, it does not re-fetch from the apiserver. Conclusion: divergence is PARTITION-BOUNDED (~45s detection + partition duration + relist latency), NOT indefinite; it self-heals when the partition ends. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `informer-cache-fresh-or-staleness-surfaced`

## Confirmed configuration and mechanism

- **0-timeout config confirmed.** `config.BindEnvAndSetDefault("kubernetes_apiserver_informer_client_timeout", 0)` — `pkg/config/setup/common_settings.go:2039`; schema default 0 — `pkg/config/schema/yaml/core_schema.yaml:9180`.
- **Flows to the informer clients.** `defaultInformerTimeout: time.Duration(...GetInt64("kubernetes_apiserver_informer_client_timeout")) * time.Second` — `apiserver.go:183`. Passed to `GetKubeClient`/`getKubeDynamicClient`/`getKubeMetadataClient`/`getKubeVPAClient`/`getCRDClient`/`getAPISClient` all as `c.defaultInformerTimeout` — `apiserver.go:402-432`.
- **Becomes rest.Config.Timeout = 0.** `GetClientConfig(timeout,...)` sets `clientConfig.Timeout = timeout` — `apiserver.go:266`. The wrap is `NewCustomRoundTripper(rt, timeout)`, and `CustomRoundTripper.RoundTrip` only *logs* on timeout — it never imposes a deadline (`roundtrip.go:37-47`). So informer watch requests carry **no client-side timeout**.

## Informer-backed decision paths (surfaces that can serve stale)

- Admission cert + webhook config informers created when `admission_controller.enabled` — `apiserver.go:460-472`. Cert is served **per TLS handshake** from the secret lister.
- Metadata controller endpoints/endpointslice informer — `controllers/metadata_controller.go:99-127` (kube-service tags served to node agents).
- Dynamic / CRD / main `InformerFactory` — `apiserver.go:439-452`.

## No post-startup freshness signal (why it fails today)

- `HasSynced` is used **only** for the one-time startup `WaitForCacheSync`: `apiserver/util.go:48,83`; admission `secret/controller.go:55,84`; `webhook/controller_v1.go:60-127`. After initial sync, `HasSynced` returns true forever regardless of whether the watch is still alive.
- No staleness metric, no readiness gate on informer liveness anywhere in these paths. The DCA README claim "serving stale data is better than serving no data" (sut-analysis §10) is honored — but **silently**, with no staleness surfaced.

## Distinction from existing catalog entries

- `admission-webhook-no-silent-nil-cert` (P2) asserts only the `(nil,nil)` cert case. This property covers the **stale-but-valid** case (e.g. old cert served after rotation, stale endpoint mapping) — a different failure the nil-cert assertion does not catch.
- `forwarder-target-is-live-endpoint` covers `GetLeaderIP`, which uses **direct** `Endpoints().Get`/`EndpointSlices().List` (leaderelection.go:282,299) plus a 5-minute local cache — not an informer. Related hazard, different mechanism.

## Open subtlety (drives confidence=medium)

Whether the cache freezes *indefinitely* depends on whether client-go's default HTTP/2 keepalive ping (ReadIdleTimeout) tears down the dead connection and forces a relist. Even if it does, the relist fails against the partition and the cache stays stale for the partition's duration — so the stale-serve-while-healthy window holds regardless; only its unboundedness is uncertain.


### from `informer-freeze-window-witnessed`

## Why the witness is needed

The root-cause property `informer-cache-fresh-or-staleness-surfaced` is a race-dependent safety invariant. Per catalog rules, a race-dependent safety invariant must be paired with a Reachable/Sometimes witness that the hazardous precondition was actually scheduled — otherwise a green AlwaysOrUnreachable that never opened the freeze window is meaningless.

## What is witnessed

Divergence between:
- **SUT lister value** — read from the frozen informer on the partitioned replica (e.g. `GetCertificateFromLister`, endpoint/metadata lister); and
- **apiserver ground truth** — a direct read the workload performs against the apiserver for the same object.

Because `HasSynced` latches true (`apiserver/util.go:48,83`) and the informer client has no client-side timeout (`kubernetes_apiserver_informer_client_timeout=0`, common_settings.go:2039 -> apiserver.go:183,266; roundtrip.go adds no deadline), a watch dropped without RST leaves the lister returning the last-known value while ground truth has moved — exactly the divergence this witness captures.

## Relationship to the root-cause property

Same fault, same instrumentation hook. This one only records that divergence occurred (Reachable); the root-cause property asserts that when it occurs while healthy, it is a violation. Running both together distinguishes 'never opened the window' from 'opened the window and the DCA silently served stale'.
