# getconfigs-distinguishes-unknown-node — Unknown-node config poll is distinguishable from a server error

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P2 · **Intent:** known-defect-reproducer

**Provenance:** merged from 1 discovery agent(s): getconfigs-500-masks-unknown-node

## Property

A node agent polling GET configs for an unknown/expired node receives a response distinguishable from a genuine internal server error, so it can re-register rather than treating a transient 500 as a hard failure.


## Invariant / assertion

Detects the ambiguity: TODAY the unknown-node branch returns HTTP 500 (dispatcher_nodes.go:33-35), identical to genuine server errors, so this assertion FAILS against current code by design — the deliverable is the reproducing trace plus the argument for a distinct code. `AlwaysOrUnreachable` on the unknown-node branch asserting the response is a distinct 4xx-style code, not 500. Its value is gated on the workload modeling node-agent backoff-on-error (a workload requirement, not a scope footnote).


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: an unknown/expired node polled GET configs.


## Antithesis angle

An unknown node → error → HTTP 500 (clusterchecks.go / dispatcher_nodes.go:33-35), the same code as genuine failures. A node whose heartbeat expired (partition) then polls GET before re-POSTing status gets a 500 and cannot tell 'you must register' from 'leader is broken.' Reorder POST/GET or expire a node then poll.


## Why it matters

Ambiguous error codes make node-agent retry/backoff logic misbehave — a node may back off hard on a benign 're-register' signal, extending cluster-check gaps. A protocol-contract clarity property.


## Mechanism refinement (from open-question investigation)

None. Core assertion confirmed against current code: unknown/expired node → HTTP 500 (dispatcher_nodes.go:34, clusterchecks.go:83), identical to genuine internal errors, so the property FAILS by design as intended (known-defect-reproducer). No distinct 4xx exists.


## Fault dependencies

- network partition / request reordering (POST vs GET; enabled by default)
- clock skew on wall-clock expiry (DISABLED by default)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable` on the unknown-node branch checking the response is the distinct code, not 500. Node-agent-side retry semantics are out of SUT scope — flag the interaction.


## Open questions (post-investigation)

- Node-agent client retry/backoff on 500 vs 4xx for GET configs — determines whether an unknown-node 500 merely delays or fully stalls config propagation. Node-agent code, out of DCA/SUT scope. `(needs human input)`
- Whether any production monitor/SLO pages on DCA 5xx rate (would make the false-500 operationally severe) — observability config, not in repo. `(needs human input)`


### Investigation Log

#### Node-agent retry/backoff on 500 vs 4xx (out of scope).

Verified the DCA-side assertion is real: dispatcher_nodes.go:32-35 returns `node %s is unknown` for !found; handler_api.go GetConfigs propagates it; clusterchecks.go:81-84 maps any error to http.StatusInternalServerError (500), indistinguishable from a genuine failure or a json.Marshal error. Not-resolvable part: the node-agent retry/backoff behavior is in node-agent code. Conclusion: core defect code-confirmed; retry impact kept needs-human.

#### Whether any monitor/SLO pages on DCA 5xx rate.

Not derivable from cluster-agent source (operational/observability configuration). Kept needs-human.

#### How does the node-agent client treat 500 vs 4xx here?

Duplicate of Q1 — same out-of-scope node-agent concern; consolidated into the single retry/backoff needs-human item.


---

## Source discovery evidence (raw, per contributing agent)


### from `getconfigs-500-masks-unknown-node`

## Property: unknown-node GET configs returns 500 (masked semantics)

### Code path
- `pkg/clusteragent/clusterchecks/dispatcher_nodes.go:28-40` — `getClusterCheckConfigs`: `node, found := d.store.getNodeStore(nodeName); if !found { return nil, 0, fmt.Errorf("node %s is unknown", nodeName) }`.
- `pkg/clusteragent/clusterchecks/handler_api.go:65-72` — `GetConfigs` returns that error verbatim.
- `cmd/cluster-agent/api/v1/clusterchecks.go:80-85` — `getCheckConfigs`: `response, err := sc.ClusterCheckHandler.GetConfigs(identifier); if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }`.
- Same file `:161-165` — `writeJSONResponse` also emits 500 on `json.Marshal` failure → the two conditions are indistinguishable to the client.

### Node expiry that makes a known node "unknown"
- `processNodeStatus` (`dispatcher_nodes.go:44-84`) auto-registers via `getOrCreateNodeStore` on POST.
- The dispatcher background loop's cleanupTicker (node_expiration_timeout/2 = 15s) expires nodes whose heartbeat is older than node_expiration_timeout (30s) and deletes the nodeStore (SUT analysis §5).
- After deletion, GET configs for that node → `!found` → 500 until the node POSTs again.

### Failure scenario
1. Node nodeA registered and receiving configs.
2. Asymmetric partition: nodeA↔leader POSTs dropped for 30s; cleanup deletes nodeA's store.
3. Partition heals for GET path first (or requests reordered). nodeA GETs `/api/v1/clusterchecks/configs/nodeA`.
4. `getClusterCheckConfigs` → not found → `http.Error(..., 500)`.
5. From the node's / operator's view this is identical to a leader crash.

### Assertion
- `assert.Reachable("getConfigs served 500 for unknown node", details)` at dispatcher_nodes.go:34 (or clusterchecks.go:83 with the error classified) — confirms the masked-semantics branch is exercised under partition/reorder faults.
- Optional stronger: tag the error type so a monitor can assert `Always(status==500 ⇒ errorKind==internal)` and watch it fail.

### Contract note
- The POST-then-GET ordering requirement is real (pull model), but surfacing the precondition failure as 500 rather than a dedicated 4xx is the protocol-contract defect. Endpoints-checks GET has the same shape (`GetEndpointsConfigs`).
