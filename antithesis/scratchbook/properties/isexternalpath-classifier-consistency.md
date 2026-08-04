# isexternalpath-classifier-consistency — Auth-path classifier matches each endpoint's intended token

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P2 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): isexternalpath-classifier-consistency

## Property

Every node-agent-facing endpoint is classified 'external' (requires DCA token) and every intra-pod IPC endpoint is classified non-external (accepts the local token), so the hand-maintained exact-segment-count classifier never mis-authorizes or silently rejects a registered endpoint.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: for each served route, isExternalPath's verdict matches the route's intended auth class. AlwaysOrUnreachable fits — evaluated per registered route (optional set), must hold whenever a route is classified. Best exercised by a workload enumerating endpoints with each token.


## Antithesis angle

isExternalPath (server.go:199-219) uses prefix + exact segment-count checks (==6, ==7); any path not matching falls back to requiring the local token, so a mis-classified new/edge endpoint silently rejects node agents (or, worse, mis-authorizes). Not fault-timing dependent — a workload sends requests with the wrong/right token to each endpoint (including trailing-slash and extra-segment variants) and asserts the expected accept/reject.


## Why it matters

A silent auth misclassification either breaks node-agent connectivity (endpoint rejects the DCA token path) or weakens the trust boundary. The brittle segment-count logic is a maintenance hazard as endpoints are added.


## Mechanism refinement (from open-question investigation)

Assertion design refined (property stands, non-vacuous): (1) ground truth for 'intended auth class' must come from client call sites (DCAClient methods → external; ipc.HTTPClient CLI → internal), not a server declaration — isExternalPath is the sole server-side source. (2) Concrete discriminating audit finding: /api/v1/info/node/{nodeName} (DCAClient.GetNodeInfo, clusteragent.go:411, used by cloudprovider.go:53) is a DCA-token endpoint absent from isExternalPath, so it is classified non-external. PLAUSIBLE live defect worth a workload probe: for a non-external path hit with only the DCA token, validateToken calls localTokenGetter first, which on failure writes http.Error 403 (util_dca.go:109/124) to the ResponseWriter BEFORE the DCA-token fallback (server.go:188-192) succeeds — so the DCA-token fallback may not cleanly mask the misclassification (response status already committed 403). (3) Classifier keys on r.URL.String() (query-inclusive) rather than r.URL.Path — latent inconsistency.


## Fault dependencies

- none required (input-domain property; not fault-timing dependent)
- needs a workload that presents both tokens against each endpoint and path variant


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` comparing isExternalPath output to a declared per-route auth class; ideally the auth class becomes a route attribute rather than a segment-count heuristic.


## Open questions (post-investigation)

None — all resolved (see Investigation Log).


### Investigation Log

#### Does any current intra-pod endpoint collide with an external prefix+count?

Audited all apiRouter routes (v1/*.go) and root IPC routes (agent.go) against isExternalPath (server.go:199-219). Found: NO intra-pod-only endpoint is currently misclassified external (the dangerous local-client-lockout direction). CLI-only routes fall through correctly: /clusterchecks (getState, 4 seg), /clusterchecks/rebalance (5 seg), /clusterchecks/isolate/check/{id} (7 seg), /endpointschecks/configs (5 seg), /tags/pod all (5 seg). Instrumentation /configs & /status (==5, external) ARE node-agent-facing (DCAClient, pkg/util/clusteragent/instrumentationchecks.go) so external is correct. One inconsistency the OTHER direction: /api/v1/info/node/{nodeName} is a genuine DCA-token endpoint (DCAClient.GetNodeInfo, clusteragent.go:411-421, called from cloudprovider.go:53) but has NO isExternalPath clause → classified non-external. Conclusion: RESOLVED — no internal→external collision today; /info/node is an external endpoint missing from the classifier (see property_change).

#### Do in-process/CLI clients ever hold the DCA token?

Examined DCA CLI subcommands: status (command.go:54 ipcfx.ModuleReadOnly + ipchttp client) and clusterchecks (pkg/cli/subcommands/clusterchecks/command.go:123,133,179 use ipc.HTTPClient) — all use the LOCAL IPC token. The DCA token (security.GetClusterAgentAuthToken, clusteragent.go:142) is held only by DCAClient (node-agent→DCA and DCA self-calls like GetNodeInfo). Conclusion: RESOLVED — CLI/local clients use the local token, NOT the DCA token, so fragility #1 (internal endpoint misclassified external → local client 403) is NOT masked; it would be a real breakage.

#### Is the query-string inclusion in r.URL.String() intentional vs r.URL.Path?

Examined server.go:180 `path := r.URL.String()` (includes query/fragment); no comment or code supports it as deliberate. buildQueryList (clusteragent.go) appends `?filter=` to real node endpoints (e.g. info/node, cf/apps), so query strings do reach the classifier and a '/' in a query value flips the segment count. ServeMux itself routes on Path. Conclusion: RESOLVED — using r.URL.String() is a latent defect, not intentional; r.URL.Path is the canonical key. The scratchbook assertion isExternalPath(Path)==isExternalPath(String()) is a valid check.

#### Is there a canonical per-route auth-class declaration, or is isExternalPath the only source of truth (tautological)?

Examined route registration (agent.go, v1/*.go): routes are registered via http.ServeMux HandleFunc with NO auth metadata. isExternalPath is the only server-side source of truth → asserting it against itself is tautological. The non-tautological ground truth is client-side: DCAClient methods (pkg/util/clusteragent/*, DCA token) = intended-external set; ipc.HTTPClient CLI callers (pkg/cli/subcommands/*) = intended-internal set. Conclusion: RESOLVED — no server-side declaration exists; the workload must derive intended auth-class from client call-site token choice.


---

## Source discovery evidence (raw, per contributing agent)


### from `isexternalpath-classifier-consistency`

## Property: isExternalPath must not misclassify intra-pod endpoints (segment-count fragility)

### Code path
- `cmd/cluster-agent/api/server.go:174-196` — `validateToken`: `path := r.URL.String()` (line 180, includes query); `if !isExternalPath(path) { try local IPC token }`; then `if !isValid { dcaTokenValidator(...) }`. So DCA token is a universal fallback; external classification only *disables* local-token acceptance for that path.
- `server.go:199-219` — `isExternalPath`: ~18 clauses of `strings.HasPrefix(path, PREFIX) && len(strings.Split(path,"/")) == N` (e.g. clusterchecks ==6, instrumentation ==5, tags/pod ==6 or ==8).

### Two concrete fragilities
1. **Internal endpoint misclassified external** → local IPC client (IPC token only) is rejected (dcaTokenValidator fails, no DCA token) → 403, fails closed and silent. Node agents are unaffected (DCA-token fallback), so it escapes node-facing tests.
2. **Query-string dependence** → `r.URL.String()` includes `?...`; a query value containing '/' changes `len(strings.Split(path,"/"))`, flipping the ==N classification for the same route. E.g. `GET /api/v1/clusterchecks/configs/nodeA?tags=a/b` splits to 7, not 6 → classified non-external. (Masked for node agents by DCA fallback, but demonstrates the classifier keys on non-canonical input.)

### Failure scenario (internal-endpoint lockout)
1. Developer adds intra-pod endpoint `GET /api/v1/instrumentation/foo` (5 segments) served on apiRouter, intended local-only.
2. isExternalPath line 212 (`/api/v1/instrumentation/` && ==5) classifies it external.
3. Local CLI/in-process client sends the IPC token only. validateToken skips local check (external), requires DCA token, client lacks it → 403.
4. Endpoint silently unreachable to its intended caller; works only for DCA-token holders.

### Assertion
- `assert.AlwaysOrUnreachable(!(isExternalPath(canonical(path)) && isRegisteredInternalOnly(path)), "internal endpoint classified external", ...)` — requires a registry of intra-pod routes.
- `assert.Always(isExternalPath(r.URL.Path) == isExternalPath(r.URL.String()), "classification depends on query string", ...)` placed in validateToken — fires whenever the query flips the segment count.
- `assert.Reachable("local IPC client rejected on intra-pod path", ...)` at the reject site to confirm the lockout is exercised.

### Scope note
- This is a static/input-domain classifier property with weak fault-timing leverage; included to cover the Protocol-Contracts focus. The DCA-token universal fallback (server.go:188-191) also means isExternalPath provides no isolation *preventing* a DCA-token holder from reaching internal endpoints — a separate contract observation.
