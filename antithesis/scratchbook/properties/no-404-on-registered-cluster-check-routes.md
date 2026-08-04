# no-404-on-registered-cluster-check-routes — Enabled cluster-check routes never return the ServeMux default 404

**Type:** Safety · **Assertion:** `Always` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 2 discovery agent(s): clusterchecks-route-404-registration-window, clusterchecks-router-404-registration-gap

## Property

Once the DCA API listener accepts connections, every node-agent-facing cluster-check route resolves to an installed handler — never the ServeMux default 404 — even in the window between Serve() starting and the routes being installed.


## Invariant / assertion

`assert.Always(no_default_404_on_clusterchecks_path)`: a request to an enabled /api/v1/clusterchecks/* path never receives the literal ServeMux '404 page not found'. Always fits — the routing guarantee must hold on every request once the listener is up. (Distinct from a handler-level 503 'startup in progress', which is a valid, intended response.)


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: a clusterchecks request arrived in the Serve()-before-route-registration window.


## Antithesis angle

The API server starts (go srv.Serve) at command.go:368, but cluster-check routes are installed later via ModifyAPIRouter at command.go:534 — after WaitForAPIClient blocks on the apiserver. A node agent hitting the endpoint in that gap gets a bare 404 (indistinguishable from a real misconfiguration) and it is a live http.ServeMux mutation (data-race concern). Inject apiserver latency to widen the WaitForAPIClient block and race node-agent polls against route registration.


## Why it matters

A 404 during the registration window makes node agents treat the endpoint as absent (vs retrying a 503), potentially disabling cluster-check polling; the concurrent mux mutation is also a latent data race. Merged from 2 focus agents.


## Mechanism refinement (from open-question investigation)

Refinement (not invalidating): the 404 gap is reachable only by an authenticated (DCA-token) node-agent request, because validateToken wraps the router and returns 401/403 before routing — assertion should fire in the auth-passed handler/wrapper and the R1 `Reachable` witness must require an authenticated /api/v1/clusterchecks/* request during the Serve()-to-ModifyAPIRouter window. Window size confirmed effectively unbounded (apiserver Backoff retrier never PermaFails).


## Fault dependencies

- network latency/congestion on DCA<->apiserver to widen the startup window (enabled by default)
- concurrency (mux mutation vs Serve)
- requires leader_election enabled


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Always` at the route handler asserting the path resolved to a real handler; the cleaner fix installs routes before Serve. A `Reachable` on the gap window confirms it is exercised.


## Open questions (post-investigation)

- Node-agent behavior on an authenticated 404 vs 503 (retry vs disable cluster-check polling) — lives in node-agent code, out of DCA/SUT scope; governs blast radius, not existence of the gap. `(needs human input)`


### Investigation Log

#### Does client-go's default 404 return before or after token validation?

Examined server.go:83 `httpHandler := validateToken(ipc)(router)` and validateToken (server.go:174-196): the mux default 404 is emitted by router/apiRouter only inside next.ServeHTTP (line 193), which runs AFTER auth succeeds. Found: an unauthenticated probe gets 401/403 from TokenValidator (util_dca.go:109/117/124), never the mux 404. Conclusion: RESOLVED — the 404 window is observable only to an authenticated caller (node agent presenting the DCA token); assert/witness must sit on the auth-passed path.

#### Exact upper bound of WaitForAPIClient under partition (max window size).

Examined apiserver.go:188-194 (retrier Strategy=Backoff, InitialRetryDelay 1s, MaxRetryDelay 5m) and WaitForAPIClient loop (apiserver.go:212-232) plus retrier.go:120-144. Found: the Backoff branch NEVER sets PermaFail (only OneTry/RetryCount do); the loop exits only on retry.OK or ctx.Done(). mainCtx is not deadline-bounded. Conclusion: RESOLVED — upper bound is effectively unbounded (whole partition duration / process lifetime); per-retry sleep capped at 5m. The 404 window width is bounded only by how long the apiserver is unreachable.

#### Node-agent behavior on authenticated 404 vs 503 (out of scope).

Not resolvable from cluster-agent source — requires node-agent config-provider retry logic (separate SUT). Kept as needs-human; it is a blast-radius question.

#### Whether a node-agent request can arrive before WaitForAPIClient returns (listener reachability vs apiserver readiness).

Examined command.go:368-376: StartServer (which opens the listener at server.go:87 and does `go srv.Serve` at server.go:150) is called at command.go:369, BEFORE apiserver.WaitForAPIClient at command.go:376. Found: the TLS listener is accepting connections before WaitForAPIClient (and thus ModifyAPIRouter at ~command.go:534) runs. Conclusion: RESOLVED — yes, a node-agent request can arrive in the gap; listener reachability strictly precedes route registration.


---

## Source discovery evidence (raw, per contributing agent)


### from `clusterchecks-route-404-registration-window`

## Mechanism (verified)

`cmd/cluster-agent/api/server.go:62` `StartServer` builds the mux tree and, at the end, launches the server in a goroutine:

- `server.go:65-66` create `apiRouter` and mount it under `/api/v1/` on `router`.
- `server.go:150` `go srv.Serve(tlsListener)` — the server is now accepting connections. `StartServer` returns immediately (`server.go:151`).

The cluster-check endpoints are NOT installed in `StartServer`. They are added much later, from the start command, via a live mutation of the already-serving mux:

- `cmd/cluster-agent/subcommands/start/command.go:369` calls `api.StartServer(...)` (server begins serving).
- `command.go:376` blocks on `apiserver.WaitForAPIClient(mainCtx)` — a hard, potentially long dependency (no client-side watch timeout; `informer_client_timeout = 0`, sut-analysis §7).
- Intervening blocking work: hostname (`:383`), `GetOrCreateClusterID` (`:433`), `StartControllers` (`:423`), `LoadComponents`/`LoadAndRun` (`:521`,`:528`).
- `command.go:530-536`: only if `cluster_checks.enabled`, after `setupClusterCheck` succeeds, does it call `api.ModifyAPIRouter(...)` → `dcav1.InstallChecksEndpoints(r, ...)`. `ModifyAPIRouter` (`server.go:155-157`) just runs `f(apiRouter)` on the live mux.

`http.ServeMux.Handle` is internally mutex-guarded, so the mutation itself is not a data race — the defect is **ordering**: for the entire interval between `go srv.Serve` and `InstallChecksEndpoints`, requests to `/api/v1/clusterchecks/*` hit the mux default and return 404.

## Failure scenario

1. DCA becomes/starts as leader with `cluster_checks.enabled=true`.
2. Antithesis injects latency or an asymmetric partition on leader↔apiserver, stalling `WaitForAPIClient` at `command.go:376`.
3. Server is live (listener open) but `InstallChecksEndpoints` has not run.
4. Node agent `POST /api/v1/clusterchecks/status/{id}` → 404 (not the expected 200/500-unknown-node contract). Node cannot register; no cluster checks dispatched to it.

## Key observations

- Distinct from a normal 404: the path is a real, will-be-registered route, so `AlwaysOrUnreachable`/`Always` on "clusterchecks path ⇒ installed handler" cleanly captures it.
- The gap also covers `/api/v2/series` and metadata routes are installed in `StartServer` so those are fine; the cluster-check and endpoints-check families are the ones added post-Serve.

## Timing window

Milliseconds under healthy startup; grows to the full apiserver-partition duration because `WaitForAPIClient` sits inside the window and has no client-side timeout.


### from `clusterchecks-router-404-registration-gap`

## Property: clusterchecks endpoints never emit the mux-default 404 once serving

### Code path
- `cmd/cluster-agent/api/server.go:64-66` — `router` and `apiRouter` created; `router.Handle("/api/v1/", http.StripPrefix("/api/v1", apiRouter))`.
- `cmd/cluster-agent/api/server.go:150` — `go srv.Serve(tlsListener)`; StartServer returns. **The listener is now live.**
- `cmd/cluster-agent/api/server.go:155-157` — `ModifyAPIRouter` mutates the *same* `apiRouter` that Serve is already dispatching against.
- `cmd/cluster-agent/subcommands/start/command.go:369` — `api.StartServer(...)` called.
- `command.go:376` — `apiserver.WaitForAPIClient(mainCtx)` **blocks** (hard startup dependency, fatal if unreachable).
- `command.go:521-528` — `common.LoadComponents`, `ac.LoadAndRun` run (more latency).
- `command.go:530-536` — only here, inside `if cluster_checks.enabled`, `api.ModifyAPIRouter(...)` installs `dcav1.InstallChecksEndpoints` → `POST /clusterchecks/status/{identifier}`, `GET /clusterchecks/configs/{identifier}`, etc. (see `cmd/cluster-agent/api/v1/clusterchecks.go:24-30`).

### Failure scenario
1. DCA process (re)starts as it becomes/So is leader; `go srv.Serve` begins accepting on cmd_port (5005).
2. A node agent's cluster-check config provider POSTs `/api/v1/clusterchecks/status/nodeA`. Auth passes (`isExternalPath` classifies it external → DCA token validates, server.go:210).
3. `next.ServeHTTP` → `router` → StripPrefix → `apiRouter`, which has **no** `/clusterchecks/...` handler yet (ModifyAPIRouter not reached — WaitForAPIClient still blocking under injected apiserver latency).
4. `apiRouter` returns the stdlib default `404 page not found`.
5. Node agent receives an authenticated 404 for a real endpoint.

### Distinguishing from legitimate 404
- Legit disabled-checks 404 is emitted by `clusterChecksDisabledHandler` (`clusterchecks.go:178-181`) with body `Cluster-checks are not enabled`, only when `sc.ClusterCheckHandler == nil`.
- `writeJSONResponse` (`clusterchecks.go:172`) emits a bodyless 404 when the marshaled payload is empty — also distinct.
- The gap 404 is the mux default (`404 page not found\n`) — that exact body for a clusterchecks path is the violation.

### Timing window
- Width ≈ time from `go srv.Serve` (server.go:150) to `ModifyAPIRouter` (command.go:534): WaitForAPIClient + LoadComponents + LoadAndRun + setupClusterCheck. Under an apiserver partition/latency fault WaitForAPIClient can take many seconds; node agents poll on a short interval, so the gap is reliably observable.

### SUT instrumentation (MISSING — net-new)
- Wrap `apiRouter`/`router` with a handler that inspects the final status+body; `assert.Always(!(isClusterchecksPath(path) && status==404 && body=="404 page not found\n"), "clusterchecks route served before registration", details)`.
- Or emit `assert.Reachable("clusterchecks routes installed")` at `command.go:535` and `assert.Always(routesInstalled || !servingClusterchecksRequest, ...)` in the auth wrapper.
