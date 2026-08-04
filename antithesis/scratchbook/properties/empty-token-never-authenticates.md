# empty-token-never-authenticates — An empty auth token never authenticates any caller

**Type:** Safety · **Assertion:** `Always` · **Priority:** P1 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): empty-token-authenticates-any-caller

## Property

The Cluster Agent never authorizes a request while its configured auth token (DCA token or local IPC token) is the empty string.


## Invariant / assertion

`assert.Always(configured_token != "" whenever a request is authorized)`: the token-compare path never succeeds against an empty configured token. Always fits — an absolute security invariant on every authenticated request.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: an authenticated request arrived while the configured token was still empty (startup window).


## Antithesis angle

Tokens are read/created at startup from a ConfigMap or file (validateToken, server.go:174-196). Inject filesystem/IO latency or error during startup, or an apiserver failure during token ConfigMap read, so the token is momentarily empty while the API server (started early, command.go:368) is already accepting connections. Assert no request is authorized against an empty token.


## Why it matters

If an empty token ever compares equal (e.g. a caller also sending an empty token, or a constant-time compare of two empty strings), the DCA's entire node-agent/IPC trust boundary collapses — any caller is authorized. A hard security floor.


## Mechanism refinement (from open-question investigation)

Core defect confirmed (property valid): TokenValidator (util_dca.go:122) authorizes `Bearer ` (empty) when the configured token is "" — tok=["Bearer",""], len==2 passes, constantCompareStrings("","")==true; gRPC path server.go:109 ConstantTimeCompare("","")==1 likewise authorizes. Reachability model corrected: the empty-token condition is NOT a startup ordering race (init precedes Serve inside StartServer); it requires InitDCAAuthToken to return an error (fault pushing FetchOrCreateArtifact past auth_init_timeout=30s, error discarded at server.go:95), and it is then DURABLE for the process lifetime, not a transient window. R1 witness should be 'authenticated request served while dcaToken=='' due to init failure'. Scope narrowed to the DCA-token surface only — the local IPC token cannot be empty in the server (NewComponent fails fast).


## Fault dependencies

- filesystem/IO latency or error injection during startup (enabled by default)
- apiserver failure during token ConfigMap read
- clock jitter (not required)


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Always(token != "")` guarding the compare in validateToken and the gRPC auth path; plus a `Reachable` on the startup window where the API server serves before the token is loaded.


## Open questions (post-investigation)

- Whether any real deployment leaves cluster_agent.auth_token unset AND the token-file directory (ConfFileDirectory, security.go:204) unwritable (read-only rootfs), making the DCA token durably empty rather than transiently — a deployment/ops call. `(needs human input)`


### Investigation Log

#### Default auth_init_timeout and how easily an IO/latency fault pushes FetchOrCreateArtifact past it.

Examined common_settings.go:1135 `BindEnvAndSetDefault("auth_init_timeout", 30*time.Second)`. FetchOrCreateAuthToken/FetchOrCreateArtifact (security.go:173-207) runs under this 30s ctx; an injected IO/latency fault exceeding 30s makes CreateOrGetClusterAgentAuthToken return ("",err) → InitDCAAuthToken sets dcaToken="" and returns err → discarded at server.go:95. Conclusion: RESOLVED — default 30s.

#### Whether the local IPC token can independently be empty during the same window.

Examined command.go:265 (DCA start uses ipcfx.ModuleReadWrite → ipcimpl.NewComponent). NewComponent (ipc.go:71-93) calls FetchOrCreateAuthToken under auth_init_timeout and RETURNS AN ERROR on failure (ipc.go:79-82) → fx graph fails → the DCA process does not start/serve. Only NewInsecureComponent (ipc.go:108-132, flare/diagnose) sets token="". Conclusion: RESOLVED — the local IPC token CANNOT be transiently empty in a serving DCA (fail-fast); the 'internal endpoints also compromised' sub-scenario is unreachable in the server process. Empty-token risk applies only to the DCA token surface, whose init error is discarded.

#### Whether any deployment leaves auth_token empty AND the token file unwritable.

cluster_agent.auth_token unset is the default (security.go:197-207 falls through to a generated file in ConfFileDirectory). Read-only conf dir → FetchOrCreateArtifact create fails → durably empty. Whether a real chart mounts that dir read-only is a deployment/ops call not encoded in agent source. Kept needs-human.

#### Does the API server accept authenticated requests before the token is populated (command.go:368 vs token init)?

CORRECTION to scratchbook premise: InitDCAAuthToken is called at server.go:95 SYNCHRONOUSLY INSIDE StartServer, BEFORE `go srv.Serve` at server.go:150. Token load is therefore ordered strictly before serving — there is NO transient race window where the server serves before init runs. Conclusion: RESOLVED — the DCA token is empty at serve time ONLY if InitDCAAuthToken FAILED (auth_init_timeout exceeded); it is then DURABLY empty (InitDCAAuthToken early-returns when dcaToken!="" at util_dca.go:38, and nothing re-invokes it), lasting the whole process lifetime.


---

## Source discovery evidence (raw, per contributing agent)


### from `empty-token-authenticates-any-caller`

## Claim
When the Cluster Agent's configured auth token is the empty string, any client presenting an empty bearer token is authenticated. This affects both the HTTP dual-token middleware and the gRPC auth interceptor.

## Mechanism (verified in source)

**Constant-time compare treats empty==empty as a match:**
```go
// pkg/api/util/util_dca.go:135
func constantCompareStrings(src, tgt string) bool {
    return subtle.ConstantTimeCompare([]byte(src), []byte(tgt)) == 1
}
```
`crypto/subtle.ConstantTimeCompare` returns 1 iff the two slices have equal length and contents. Two empty slices (len 0, equal contents) → returns 1. So `constantCompareStrings("","") == true`.

**HTTP validator (pkg/api/util/util_dca.go:102-128):** `Authorization: Bearer ` splits to `["Bearer", ""]` (len==2), so the `len(tok)!=2` guard passes; then `constantCompareStrings(tok[1]=="", tokenGetter()=="")` is true → no error returned → request authorized.

**gRPC interceptor (cmd/cluster-agent/api/server.go:108-114):**
```go
if subtle.ConstantTimeCompare([]byte(token), []byte(util.GetDCAAuthToken())) == 0 {
    return struct{}{}, errors.New("Invalid session token")
}
return struct{}{}, nil  // <- empty token vs empty configured token: compare==1, not 0, so NO error
```

**Token can be empty at serve time (verified):**
- `dcaToken` is a package var defaulting to `""` (util_dca.go:27).
- `GetDCAAuthToken()` returns it under RLock (util_dca.go:51-55).
- `InitDCAAuthToken` (util_dca.go:33-48) sets `dcaToken, err = CreateOrGetClusterAgentAuthToken(ctx,...)` under a `auth_init_timeout` context; on error `dcaToken` is the returned zero value `""` and the error is propagated up.
- `api.StartServer` calls it at server.go:95 as `util.InitDCAAuthToken(...) //nolint:errcheck` — **the error is discarded** — and then proceeds to build and serve (`go srv.Serve`, server.go:150).
- Startup ordering: `StartServer` at command.go:369 runs BEFORE `apiserver.WaitForAPIClient` at command.go:376 ("main API server started early to ease investigations", per SUT analysis §2).

## Failure scenario
1. DCA boots; `cluster_agent.auth_token` unset in config, so the token is created/fetched from the filesystem via `FetchOrCreateArtifact` under a `auth_init_timeout` context (security.go:196-214).
2. Antithesis injects filesystem/IO latency (or clock jitter) so the create/fetch exceeds `auth_init_timeout`; `CreateOrGetClusterAgentAuthToken` returns `("", err)`.
3. `InitDCAAuthToken` sets `dcaToken=""` and returns err; server.go:95 discards the err; the server serves anyway.
4. Attacker (any pod with network reach to cmd_port, no valid credential) sends `GET /api/v2/series` with `Authorization: Bearer ` (empty) or opens a gRPC tagger stream with an empty token.
5. `constantCompareStrings("","")==true` → **authenticated**. Full bypass until the token is repopulated (which requires a restart, since `InitDCAAuthToken` is a no-op once set and there is no re-init path).

## Key observations
- The `validateAuthToken` min-length guard (security.go:216-221) is only applied on the *successful* config/filesystem return paths, NOT on the error path where `dcaToken` becomes `""`.
- Both auth surfaces (HTTP + gRPC) share the empty-compare defect.
- Note the HTTP middleware's fallback design (server.go:183-193): internal paths try the local token then fall back to the DCA token, so an empty DCA token compromises *internal* endpoints too when the local token also happens to be empty during the same init window.

## Timing window
The entire interval between `go srv.Serve` (server.go:150) and a (nonexistent) successful re-init — in practice the whole process lifetime if init failed, since re-init is a no-op guarded by `if dcaToken != ""`.
