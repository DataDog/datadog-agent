# forwarder-request-fidelity — Forwarded request preserves path and Authorization

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 3 discovery agent(s): forwarder-forwarded-path-fidelity, forwarder-preserves-auth-header, forwarder-stripped-prefix-forward-witness

## Property

When a follower forwards a node-agent request to the leader, the outbound request's URL path equals the original request target (the full /api/v1/... or /api/v2/... path), not the StripPrefix-stripped path.


## Invariant / assertion

In the ReverseProxy Director (leader_forwarder.go:123-135), after path restoration the outbound req.URL.EscapedPath() equals url.ParseRequestURI(req.RequestURI).EscapedPath(), and it begins with a registered API prefix (/api/v1 or /api/v2). Equivalently: ParseRequestURI(req.RequestURI) succeeded (err==nil) AND req.URL.Path/RawPath were set from it. `AlwaysOrUnreachable` fits — the forward path is optional (only followers forward), but any forwarded request must preserve its path and Authorization.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Sometimes`: At least once, a follower forwards to the leader a request whose incoming URL.Path had been stripped (differs from the RequestURI path), proving the StripPrefix->restore interaction was genuinely scheduled and the fidelity invariant did not pass vacuously.


## Antithesis angle

The follower's leader-proxy handlers are registered UNDER http.StripPrefix (server.go:66,79), so by the time Forward runs, req.URL.Path is already stripped to e.g. /clusterchecks/status/{id} while req.RequestURI still holds /api/v1/clusterchecks/status/{id}. The Director is the ONLY code that restores the prefix, and it does so behind `if err == nil` (line 130): any RequestURI that fails url.ParseRequestURI silently falls through, forwarding the STRIPPED path — the leader then 404s or routes to the wrong handler. A percent-encoded segment (check digest, node name, tag with %2F or reserved chars) can also mismatch if RawPath is not carried faithfully. Antithesis reaches this only by running >=2 replicas with real leader election so the follower->leader forwarding path actually executes through the real StripPrefix router; existing unit tests stub the target and set req to /foo, never exercising restoration. A fuzzing workload that sends edge-case request targets (percent-encodings, trailing slash, double slash, dot segments, semicolon params) through a follower drives the parse/escape edges.


## Why it matters

A silent mis-route sends an authenticated node-agent request to the wrong leader handler or yields a 404/500 while the operator sees the DCA as healthy. Cluster-check config pulls (GET /api/v1/clusterchecks/configs/{id}) or heartbeats (POST .../status/{id}) that mis-route stop dispatching checks for that node — a cluster-wide data-plane outage with no error surfaced on the leader. This restoration logic is new (net/http refactor #50380) and untested for its actual purpose.


## Fault dependencies

- leader_election enabled + >=2 replicas (required; forwarding path is inert otherwise)
- A workload that drives node-agent requests at a FOLLOWER replica so the follower->leader forward executes (required)
- NO node-termination and NO clock-skew needed; a workload that sends edge-case request targets (percent-encoded segments, trailing/double slash, dot segments) maximizes coverage of the parse/escape branch


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

In the Director closure (leader_forwarder.go:123-135), after the restoration block, add: `parsed, err := url.ParseRequestURI(req.RequestURI); assert.AlwaysOrUnreachable(err == nil && strings.HasPrefix(req.URL.EscapedPath(), "/api/v") && req.URL.EscapedPath() == parsed.EscapedPath(), "leader-forwarded request retains full un-stripped API path", map[string]any{"requestURI": req.RequestURI, "outPath": req.URL.EscapedPath(), "parseErr": fmt.Sprint(err)})`. Requires adding github.com/antithesishq/antithesis-sdk-go to the root module go.mod (no SDK currently present).


## Open questions (post-investigation)

- Whether the leader's net/http ServeMux treats a Path/RawPath mismatch as a different route: the assertion keys on EscapedPath equality, which is what Go 1.22 ServeMux matches on, so equality is the right check; residual is confirming the leader mux uses default 1.22 matching with no custom normalization. `(partial)`


### Investigation Log

#### Q1: Can a legitimate node-agent request make url.ParseRequestURI return err?

Reasoned about net/http server RequestURI semantics and the routes in isExternalPath (server.go:200-213). Found: node-agent calls use origin-form request targets (/api/v1/clusterchecks/..., /api/v2/series), which ParseRequestURI parses with err==nil. err is produced only for authority-form (CONNECT) or asterisk-form (OPTIONS *) or malformed targets. Concluded: the silent-fallthrough (err!=nil) branch is reachable only via a hostile/fuzzed client the workload must synthesize, not by legitimate traffic. RESOLVED.

#### Q3: Are there leader-proxied handlers on the ROOT router (not under /api/vN StripPrefix)?

Examined server.go:64-80,154-162 and grepped ModifyAPIRouter/ModifyRootRouter callers. Found: apiRouter is mounted under StripPrefix("/api/v1") and v2ApiRouter under StripPrefix("/api/v2"); all leader-proxied handlers (clusterchecks/endpointschecks via v1, languagedetection via apiRouter, series via v2ApiRouter) register there. ModifyRootRouter has ZERO callers; ModifyAPIRouter callers (command.go:534,771) register on apiRouter. Concluded: no leader-proxied handler is on the root router, so path restoration never over-prepends a prefix. RESOLVED.

#### Q4: What exact header carries the DCA token?

Examined validateToken (server.go:174-196 → util.TokenValidator/GetDCAAuthToken) and server_test.go:69. Found: token carried as `Authorization: Bearer <token>`. Concluded: the assertion must key on the Authorization header. RESOLVED.

#### Q5: Is there middleware that could strip Authorization?

Examined the only middleware wrapping the router: validateToken (server.go:83, 174-196) and RecoveryHandler (line 143). Found: validateToken reads Authorization for token validation and calls next.ServeHTTP(w, r) with the request unchanged (no Header.Del anywhere in the chain). Concluded: no middleware strips Authorization; the auth-preservation invariant is non-vacuous and holds. RESOLVED.

#### Q6: Which leader-proxied endpoints most reliably fire the witness early?

Examined clusterchecks.go:25-29. Found the highest-traffic follower-forwarded routes: GET /clusterchecks/configs/{identifier} and POST /clusterchecks/status/{identifier} (node-agent config pulls and heartbeats, both under /api/v1 StripPrefix). Concluded: drive these two endpoints at a follower to fire the stripped-path witness earliest. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `forwarder-forwarded-path-fidelity`

## Mechanism (verified from source)

- `cmd/cluster-agent/api/server.go:66,79` register the v1/v2 API routers behind `http.StripPrefix("/api/v1", ...)` / `http.StripPrefix("/api/v2", ...)`. StripPrefix rewrites `req.URL.Path` but, per its doc and the code comment at `leader_forwarder.go:127-129`, leaves `req.RequestURI` untouched.
- The leader-proxy handlers (`cmd/cluster-agent/api/v1/languagedetection`, `.../v2/series`, and clusterchecks `handler_api.go:35`) run inside those stripped routers. When the node is a follower, `LeaderProxyHandler.rejectOrForwardLeaderQuery` (`leader_handler.go:108-133`) calls `LeaderForwarder.Forward` with the already-stripped `req`.
- The Director (`leader_forwarder.go:122-135`) restores the path:
  ```go
  if u, err := url.ParseRequestURI(req.RequestURI); err == nil {
      req.URL.Path = u.Path
      req.URL.RawPath = u.RawPath
      req.URL.RawQuery = u.RawQuery
  }
  ```
  The `if err == nil` guard means a parse failure SILENTLY skips restoration, leaving the stripped `/clusterchecks/...` path on the outbound request. The leader's own StripPrefix router then fails to match `/api/v1` and the request 404s or hits the wrong handler.

## Why this is Antithesis territory (not just a unit test)

- The three existing tests in `pkg/clusteragent/api/leader_forwarder_test.go` build the request with `httptest.NewRequest("GET", "http://example.com/foo", nil)` — no StripPrefix, no `/api/v1` prefix, and they assert only status code + the forward header at the leader, never the received PATH. The restoration branch is dead in unit tests.
- Exercising it requires the real two-replica, leader-elected topology so the follower->leader forward runs through the production StripPrefix router. That is the state only the live system (Antithesis) reaches.
- `git log` confirms the restoration was introduced in the recent gorilla/mux -> net/http refactor `d85a724234f` (#50380, 2026-05-19). New, load-bearing, and untested for correctness.

## Escaping sub-hazard

`url.ParseRequestURI` sets `Path` (decoded) and `RawPath` (only when it differs from the default encoding). The Director copies both, and `httputil.ReverseProxy` emits `req.URL.EscapedPath()` on the wire. A check digest or node name containing `%2F` / reserved characters is the input class most likely to expose a Path/RawPath inconsistency that silently changes which handler the leader dispatches to.

## Instrumentation intent

Instrument inside the Director so the assertion evaluates on every real forward. Guard the assertion so it is meaningful only when the forwarding path actually ran (see the paired witness `forwarder-stripped-prefix-forward-witness`).


### from `forwarder-preserves-auth-header`

## Mechanism

- The leader gates every request through `validateToken(ipc)` (`server.go:83`); node-agent (external) paths require the DCA auth token, carried as an `Authorization` header.
- On the follower, `LeaderForwarder.Forward` (`leader_forwarder.go:86-110`) hands the *same* `*http.Request` to the ReverseProxy. The Director mutates only `URL.Scheme/Host`, the path fields, and `Header.Add(forwardHeader,"true")` — it never touches `Authorization`. Go's `httputil.ReverseProxy` copies inbound headers to the outbound request by default (it removes only hop-by-hop headers).
- Therefore preservation holds by construction on the current code.

## Honest value assessment

This is close to unit-test territory: a single test that sends a request with an `Authorization` header through `Forward` to a capturing leader would confirm today's behavior. The existing tests (`leader_forwarder_test.go`) assert the ADDED `X-DCA-Follower-Forwarded` header at the leader but do NOT assert the inbound credential survives — so there is a real, if small, coverage gap. The Antithesis-only marginal value is exercising it end-to-end through the real StripPrefix + leader-election path (shared with the two path-fidelity properties) so a regression in either the Director or an upstream middleware is caught in the running system rather than only in a stub.

## Recommendation

Ship this assertion co-located with the path-fidelity instrumentation (same Director, same run) at near-zero incremental cost, but do NOT spin up a dedicated fault scenario for it. If harness budget is tight, a Go unit test covering Authorization pass-through is an acceptable substitute for this one (unlike the path-fidelity property, which genuinely needs the live topology).


### from `forwarder-stripped-prefix-forward-witness`

## Purpose

Paired reachability witness for `forwarder-forwarded-path-fidelity`. The invariant is `AlwaysOrUnreachable`, so a green result is only meaningful if the stripped-path forwarding window was actually scheduled.

## Precondition being witnessed

- Two+ replicas with leader election on; the request lands on a FOLLOWER.
- The handler is registered under `http.StripPrefix` (server.go:66,79), so on entry to `Forward` the incoming `req.URL.Path` is the stripped form (e.g. `/clusterchecks/status/{id}`) while `req.RequestURI` is the full `/api/v1/clusterchecks/status/{id}`.
- The two differ exactly when the prefix was stripped — that difference is the signal restoration is load-bearing.

## Distinguishes from vacuous pass

Existing unit tests never trip this: they use `/foo` with matching RequestURI and no StripPrefix, so `req.URL.Path == ParseRequestURI(req.RequestURI).Path` and the witness would stay unproven — correctly flagging that the real path was never exercised in unit testing.
