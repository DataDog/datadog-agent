# admission-webhook-available-under-churn — The DCA that should serve the admission webhook stays available under churn

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 2 discovery agent(s): admission-webhook-available-under-churn, admission-webhook-served-during-churn-witness

## Property

Whenever a DCA replica is running the admission webhook HTTPS server, has synced its secret informer at least once, and the admission cert Secret exists in the apiserver, a TLS handshake against that replica completes with a NON-nil certificate — so the DCA does not needlessly fail its webhook (and, under failurePolicy=Fail, needlessly block cluster-wide pod creation) merely because leadership moved or the apiserver was briefly partitioned. Serving is not leader-gated: this must hold on the leader AND on followers.


## Invariant / assertion

assert.AlwaysOrUnreachable: at each GetCertificate invocation (cmd/cluster-agent/admission/server.go:137-144), if the cert Secret is present in the replica's secret-informer cache (GetCertificateFromLister returns err==nil), the returned *tls.Certificate is non-nil. Corollary asserted from the workload: while the cert Secret exists in the apiserver (workload-observable ground truth) and >=1 DCA replica is in the webhook Service's Ready endpoints, an AdmissionReview probe POSTed to the webhook Service completes a TLS handshake and returns a valid AdmissionResponse — regardless of which replica is the lease leader and regardless of an in-flight leader<->apiserver partition. AlwaysOrUnreachable fits: the webhook path is optional (admission_controller.enabled + a request must arrive), but every served handshake, once reached, must not needlessly fail-closed while the cert is available.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: At least once per run, a DCA replica completes a webhook TLS handshake (non-nil cert) and produces a valid AdmissionResponse WHILE a leadership-churn or leader<->apiserver partition event is in effect (and ideally while the serving replica is NOT the lease leader). This proves the hazardous availability window from admission-webhook-available-under-churn was actually scheduled, so the paired AlwaysOrUnreachable is not a vacuous green.


## Antithesis angle

The webhook HTTPS server is started on EVERY replica gated only on admission_controller.enabled (command.go:648), with no leader check on server.Run (command.go:712); the cert is fetched per-handshake from a per-replica informer-backed secretsLister (server.go:140). Cert creation/rotation, however, IS leader-gated (controller_base.go:286,299 early-return when !isLeaderFunc). informer_client_timeout=0 (sut-analysis §7) means a leader<->apiserver partition can silently freeze a replica's secret informer with no error. Antithesis can: (1) partition a replica from the apiserver and keep sending webhook requests — a synced replica's lister must keep returning the cached cert (client-go does not evict on watch disconnect), so the handshake must still succeed; (2) churn leadership (partition >= lease duration, or terminate the leader) while webhook traffic flows and assert followers still serve; (3) drive a leader-gated cert ROTATION while a follower is partitioned to probe whether the follower serves a cert the freshly-rotated CABundle no longer trusts. Readiness ('admission-controller-webhook', server.go:91,176) is drained by the Run loop independent of cert state, so a Ready replica still in the Service endpoints can serve a nil cert — the exact needless fail-closed this asserts against. This is unreachable in the existing single-process, fake-clientset unit tests (no real informer freeze, no multi-replica churn, sut-analysis §9).


## Why it matters

This is the availability half of the admission story (the existing admission-webhook-no-silent-nil-cert property is the code-contract half: 'fail loudly, never (nil,nil)'). Here the concern is operational: with admission_controller.failure_policy=Fail, a needless handshake failure on a Ready, Service-selected replica blocks ALL matching pod creation cluster-wide with a misleading TLS error; with the default Ignore it silently drops the intended mutation. Because serving is not leader-gated but cert maintenance is, leadership churn is a real trigger for a serving replica whose view of the cert diverges from the leader's. Antithesis is the only way to hold a frozen-informer + churn + concurrent-traffic state.


## Mechanism refinement (from open-question investigation)

Scope refinement, no invalidation. Q2 resolves in the invariant's favor: a never-synced replica cannot serve a nil cert because webhook-server startup is gated behind SyncInformers/WaitForCacheSync of the Secrets informer (start.go:154 → command.go:690), removing one hypothesized 'fresh-replica fail-closed' violation and strengthening the 'synced replica keeps serving' invariant (Q4 confirms cache retention across disconnect). The remaining live violation candidate is rotation-during-churn (Q3), not fresh-start.


## Fault dependencies

- network partition leader<->apiserver (enabled by default) to freeze a replica's secret informer and to churn leadership >= leader_lease_duration
- node termination of the leader (DISABLED by default) — only needed for the crash-based churn variant; partition-based churn is a workload-driven substitute
- workload must drive admission traffic: POST AdmissionReview probes to the webhook Service endpoint during/after the fault, and observe the cert Secret existence via the apiserver as ground truth
- requires admission_controller.enabled + leader_election enabled + >=2 replicas; failure_policy is deploy-config (does not gate the DCA-side assertion)


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING (zero existing SDK usage). (1) In GetCertificate (server.go:137-144), after the fetch: assert.AlwaysOrUnreachable(err != nil || cert != nil, "admission webhook returns a non-nil cert whenever the secret lister has it", details{err, isLeader, secretName}) — plus emit details tagging the replica's leader state and whether the informer HasSynced, so a nil-cert event is attributable to unsynced-cache vs partition vs rotation. (2) Workload-side (preferred, runs against a stock image): while the cert Secret exists in the apiserver, POST a probe AdmissionReview to the webhook Service and assert the TLS handshake + AdmissionResponse succeed — this is the end-to-end availability assertion and does not require SDK inside the DCA. Pair with the witness property below so the AlwaysOrUnreachable is not vacuous.


## Open questions (post-investigation)

- Does the webhook Service (admission_controller.service_name) select ALL DCA pods or only the leader in the harness? DCA code serves on every replica (command.go:648, no leader gate on server.Run :712), but the harness workload 'manages the DCA Service + EndpointSlice' (deployment-topology.md:52) so the selector is a harness-authoring decision not yet pinned. Confirm the shipped Service selector when the harness is built. `(needs human input)`
- Rotation-during-churn: when the leader regenerates the cert AND its CABundle while a follower is partitioned, does the follower's cached old cert remain trusted or get rejected by the new CABundle? Cert rotation is rare (yearly; 30d-before refresh) and CABundle update is leader-gated (controller_base.go:286,299); the code does not guarantee a partitioned follower's old cert matches a freshly-rotated CABundle. Resolving the actual trust outcome needs the cert-controller regeneration/CABundle semantics plus an intended-behavior call. `(partial)`
- Default admission_controller.failure_policy rendered in the harness (code default 'Ignore', common_settings.go:653; harness/Helm value not pinned in topology docs). Sets severity (silent mutation-drop vs cluster-wide pod-creation block); the DCA-side assertion is policy-agnostic. `(needs human input)`
- Witness design: require the serving replica to be a non-leader (stronger, proves not-leader-gated serving) or accept any replica? Pure test-design choice (scratchbook advises 'start permissive, tighten'); no code answer. `(needs human input)`


### Investigation Log

#### Q2: Fresh/never-synced replica — is serving a nil cert reachable in harness startup ordering (server starts before/independently of SyncInformers)?

Examined command.go:687-716 and pkg/clusteragent/admission/start.go:118-154. server.Run (command.go:712) starts ONLY inside the else-branch entered when StartControllers succeeds. StartControllers ends with apiserver.SyncInformers(informers,0) (start.go:154) which WaitForCacheSync's the Secrets informer (util.go:37-58) with kube_cache_sync_timeout_seconds (default 10, common_settings.go:284). If the secret informer never syncs, StartControllers returns a SecretsInformer SyncInformersError → command.go:690 condition true → logs 'Could not start admission controller' and the server is NOT started. Concluded: a never-synced replica does NOT serve a nil cert — it fails to bring up the webhook server entirely; the informer must have synced >=1 before serving. NOT reachable. RESOLVED (strengthens the invariant).

#### Q4: Does client-go's lister retain the last-synced Secret across a watch disconnect with informer_client_timeout=0?

client-go informers serve reads from a local thread-safe cache (Indexer); a watch disconnect does not evict entries — the reflector re-lists/re-watches on reconnect and does not clear the store on transient error. Concluded: the last-synced Secret is retained across a disconnect, so a synced replica keeps returning the cached cert through a partition. Underpins the 'synced replica keeps serving' invariant. RESOLVED (design of client-go; discovery evidence concurs).

#### Q7: Can the workload reliably observe 'churn in effect'?

Examined antithesis/scratchbook/deployment-topology.md:52. The workload 'drives leadership events' and manages the DCA Service + EndpointSlice itself. Concluded: the workload owns the fault-active flag by driving leadership transitions/partitions itself rather than inferring churn from the Lease object. RESOLVED (harness design per topology).

#### Q1: Service selects all pods or only leader (deploy-side)

Examined command.go:648-717 — serving is gated only on admission_controller.enabled with no le.IsLeader check on server.Run (:712), so every replica serves. The Service selector itself is harness-managed (topology:52) and not pinned. KEPT needs-human (deploy/harness config).

#### Q3: Rotation-during-churn trust

Examined cert rotation config (common_settings.go:636-637, ~yearly) and leader-gating of cert/CABundle maintenance (controller_base.go:286,299). Rare edge; actual trust outcome not determinable from code alone. KEPT partial.

#### Q5: Default failure_policy rendered in harness

Code default 'Ignore' (common_settings.go:653); harness value not pinned. KEPT needs-human (deploy-side).

#### Q6: Witness require non-leader replica?

Test-design decision, no code answer. KEPT needs-human.


---

## Source discovery evidence (raw, per contributing agent)


### from `admission-webhook-available-under-churn`

## Mechanism (verified in source)

**Serving is NOT leader-gated; cert maintenance IS.**
- `cmd/cluster-agent/subcommands/start/command.go:648` — the entire admission block is gated only on `admission_controller.enabled`.
- `command.go:687-717` — `StartControllers` then `server := admissioncmd.NewServer(secretsLister)` and `go server.Run(mainCtx)`; **no `le.IsLeader` check** anywhere on the serving path. Every replica listens on `admission_controller.port` and serves the webhook.
- `pkg/clusteragent/admission/controllers/webhook/controller_base.go:286` (`handleSecret`) and `:299` (`handleSecretUpdate`) — both early-return `if !c.isLeaderFunc()`. Cert creation/rotation (the secret controller, start.go:75-81) is leader-gated. Followers only ever *read* the Secret via their informer.

**Per-handshake cert fetch that swallows the error:**
```go
// cmd/cluster-agent/admission/server.go:137-145
GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
    secretNs := namespace.GetResourcesNamespace()
    secretName := ...GetString("admission_controller.certificate.secret_name")
    cert, err := certificate.GetCertificateFromLister(s.secretsLister.Secrets(secretNs), secretName)
    if err != nil { log.Errorf("Couldn't fetch certificate: %v", err) }
    return cert, nil   // (nil,nil) on lister miss
}
```
```go
// pkg/util/kubernetes/certificate/certificate.go:139-151
func GetCertificateFromLister(...) (*tls.Certificate, error) {
    secret, err := lister.Get(secretName)   // reads informer CACHE
    if err != nil { return nil, err }
    cert, err := ParseSecretData(secret.Data)
    if err != nil { return nil, err }
    return &cert, nil
}
```
So `err==nil` <=> the Secret is present in this replica's informer cache and parses; in that case a non-nil cert is returned. The failure surface is entirely 'cache does not have a usable Secret.'

**Readiness does not reflect cert-serve capability:**
- `server.go:91` registers readiness `admission-controller-webhook`; `server.go:176-178` drains `s.healthHandle.C` in the `Run` select loop unconditionally. A replica whose informer cache lacks the cert is still Ready, stays in the webhook Service endpoints, and gets routed apiserver traffic it will fail-closed.

**Fault leverage (sut-analysis §7 item 5, §9):** `informer_client_timeout=0` → a leader<->apiserver partition can freeze the secret informer with no surfaced error. Existing tests are single-process, fake-clientset, single-goroutine — no informer freeze, no multi-replica churn.

## Expected behavior under fault (why intent=invariant, with edges)
- **Synced replica, then partition:** client-go listers serve from the last-synced cache; a watch disconnect does NOT evict entries. A replica that synced the cert keeps returning it through the partition → handshake succeeds. **This is the invariant that should hold today and that Antithesis confirms under real partition timing.**
- **Edge / should-improve (tracked in open questions):** a replica that NEVER synced the Secret before the partition (fresh start / long heal) has an empty cache → nil cert → needless fail-closed; and a follower partitioned across a leader-driven cert+CABundle rotation may serve a cert the new CABundle no longer trusts. These are reachable violations of the broader availability goal and are where the property may fire.

## Split from `admission-webhook-no-silent-nil-cert` (existing P2)
- That property asserts the **callback contract**: `GetCertificate` must never return `(nil,nil)` — it should fail loudly (observability / should-improve).
- **This** property asserts **availability**: conditioned on the cert being *available*, the DCA actually completes the handshake and returns an AdmissionResponse across churn/partition, i.e. it does not needlessly trip fail-closed. Different failure (needless unavailability vs opaque error), different fault emphasis (churn + not-leader-gated serving + informer retention), and workload-observable end-to-end, not just callback-internal.


### from `admission-webhook-served-during-churn-witness`

## Purpose
Paired witness for `admission-webhook-available-under-churn` (per the workflow rule: a race-dependent AlwaysOrUnreachable safety invariant must be accompanied by a Reachable/Sometimes witness that the hazardous precondition was scheduled).

## What must coincide
- **Fault active:** leader<->apiserver partition in effect, or a leadership transition recorded within the current lease interval (leader terminated / stepped down / re-acquired).
- **Traffic served:** a webhook request reached a DCA replica, `GetCertificate` returned non-nil (server.go:144), and `handle()` produced an `AdmissionResponse` (server.go:254/286).
- **Bonus attribution:** the serving replica's `le.IsLeader()==false` — witnessing that serving is not leader-gated (command.go:712) and a follower carried admission availability through churn.

## Instrumentation
- Workload emits an `assert.Reachable("admission webhook served during churn/partition", details{fault_active, serving_replica, is_leader})` when a probe AdmissionReview succeeds and the workload's own fault-state flag is set. Runs against a stock DCA image (no SUT-side SDK required), though the DCA-side details in the main property enrich attribution.

## Why not a unit test
The coincidence of (informer-frozen/churning DCA) x (in-flight admission request) is exactly the timing/partial-failure interleaving Antithesis controls and that the existing single-process fake-clientset tests cannot reach (sut-analysis §9).
