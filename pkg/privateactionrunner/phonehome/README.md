# RC-first PAR phone-home POC — three-PR stack

This opt-in experiment runs two owners inside core: **RC owns discovery; PAR
owns launch and enrollment reconciliation**. Shipping defaults are unchanged.
It is not fleet rollout, an rc-edge extension, or a revocation policy.

## Review slices and reuse

1. `jules.macret/par-rc-status` (base `main`): immutable RC org/key status from
   the existing poller; no extra requests. Remote Config review boundary.
2. `jules.macret/par-rc-gated-phone-home` (base PR 1): modern
   `comp/privateactionrunner/phonehome/{def,impl,fx}` plus a typed status reader,
   demand and RC-gated supervisor calls. Production PAR scope is intentionally
   unknown: this slice alone cannot fresh-enroll. Action Platform / Agent Runtimes
   / Remote Config review boundaries.
3. `jules.macret/par-rc-aaa-discovery` (base PR 2): RC-side AAA producer plus
   HTTP-to-reader-to-controller integration tests. Remote Config owns the HTTP
   client, parser, schedule and lifecycle. Coordinate load contracts with AAA;
   no backend change is included.

The original POC remains at `c2ed73ae401` on `jules.macret/par-phone-home-poc`,
draft #57623. PR 2 reuses its enrollment/OPMS journal, protected identity storage,
component retention, status and safety tests. PR 3 moves its `discovery.go` and
HTTP parser tests to `pkg/config/remote/service/par_discovery*.go`, adapting the
schedule to the RC-owned lifecycle. PAR has **no discovery HTTP client/fallback**.
The old RC-independent startup policy is superseded, including enrolled restarts.

## Gate, ownership and lifecycle

- Existing `/api/v0.1/status` requests supply independent `enabled`/`authorized`
  facts. Freshness is two configured status intervals, capped at five minutes,
  measured from request start. Ordinary RC cadence/delivery/diagnostics are unchanged.
- Every AAA request and every POC-controlled Start requires a fresh matching RC
  observation with **both booleans true**. Unknown, unavailable, locally disabled,
  fetch failure, stale, false and credential/site mismatch close the gate. Nothing
  enables RC implicitly. Its ordinary poll detects later grants without restart.
- RC's opt-in worker is attached by the core RC component, not every RC library
  consumer. Demand is necessary, not sufficient: HTTP dispatch checks readiness
  again. The Agent transport keeps proxy/TLS support; redirects are refused.
- AAA `GET /api/v2/validate` uses only the effective enrollment API key. It requires
  `valid: true` and `private_action_runner_enroll`, not `remote_config_read` or just
  HTTP 200. Bounded categories distinguish missing scope, invalid credentials,
  malformed/oversized/trailing data, HTTP failure and throttling; no raw errors or
  response bodies are exposed by this discovery path.
- RC and AAA have separate timestamps, private key/endpoint bindings and an RC
  continuity generation. Failed/stale RC checks or credential changes invalidate
  older grants. An in-flight AAA request is canceled on demand release/shutdown,
  or within the one-second prerequisite watch on readiness loss. Late results
  cannot restore a grant after close/reopen. Reads also check current readiness,
  generation and freshness, even between worker ticks. AAA grants expire after
  two minutes, measured from request start.
- AAA startup jitter is 0–60 seconds. Normal/missing-scope polling is 60–72 seconds;
  transient failures use 1/2/4/8/15-minute bases plus 0–20% jitter. Auth/other HTTP
  failures wait 15 minutes plus jitter. GET 429 honors delta/date Retry-After,
  minimum/fallback one minute, plus nonnegative jitter. Saturated hints cannot
  overflow when jitter is added. In-memory deadlines survive gate/demand toggles,
  not a whole core restart; reopening cannot force a request or reset backoff.
- Core PAR reads observations, serializes launch authority and checks RC again
  immediately before Start. Go still owns POST, keys and identity persistence.
  Stop discovery demand only after confirmed persisted enrollment, not a PID.
  Ordinary RC polling continues. Core startup does not wait for any prerequisite;
  both owners cancel background work during shutdown.
- Reusable identities and storage-only recovery bypass a new AAA scope check,
  **not RC's gate**. Healthy running PARs and credential holders are not killed
  merely because readiness is lost. A new recovery helper waits for RC.

## Supported isolated deployment

`DD_PAR_PHONE_HOME_POC=true` must be set in core and executor. Use a locally enabled,
non-FIPS, non-containerized Linux node Agent in split mode, with matching main
config, resolved secrets, environment, hostname, user, local filesystem and an
absolute identity path. Extra PAR config, OPMS extra headers and live key/site
rotation remain unsupported. Both sources must match the enrollment key/site;
RC overrides cannot silently send that key to a different discovery endpoint.
Internal endpoint override is allowed only for paired loopback RC/OPMS endpoints.
The reader freezes startup key/endpoints, including loopback endpoints: changes
cannot bypass RC's gate for reusable identities. POC opt-in is rechecked at dispatch.

Set an explicit absolute `DD_PM_SOCKET_PATH`. Register
`datadog-agent-par-control` and `datadog-agent-action-executor` with
`auto_start: false` and `restart: never`; executor registration explicitly includes
`env: {DD_PAR_PHONE_HOME_POC: "true"}`. Core refuses conflicting policies and only
uses Describe/Start/Stop: no shell, Create, or policy rewriting. Shipping package,
Docker, Helm and Operator templates remain unchanged. `testdata/processes.d`
contains examples, not a complete real-process harness.

## Enrollment recovery and safety limits

Durable enrollment deadlines are independent of AAA: verified quota/auth/scope
no-creation rejections retry with 5/10/20/30-minute bases plus 0–20% jitter,
subject to any later Retry-After. Successful AAA checks and RC reopening cannot
shorten them. POST auth/scope failures save a revalidation floor; the cached grant
from before rejection cannot authorize replay. RC owns the next AAA request.

Protected 0600 `<identity>.pending-<attempt-ID>` files save the key/name before
POST, then the returned URN before active publication. A separate 0700 journal
contains exclusive synced POST claims, bounded outcomes and deadlines, never
credentials. Atomic identity publication and durable outcome acknowledgement
precede success. A visible identity file by itself is not acknowledgement.

Storage-only recovery reuses the same key/URN/attempt, with 1/2/4/8/15-minute bases
plus jitter and no second POST. If credentials **or the recovery outcome** cannot
be safely saved, the Go helper remains alive, retries local storage each minute,
and serves an unavailable—not falsely disabled—bootstrap snapshot. It is not
an action executor. Do not kill it to claim zero RSS. Go resolution/POST keep
45/30-second limits and Rust retains its 120-second bootstrap budget.

Unknown/bare OPMS 403 gates, unstructured auth, enrollment 429, 5xx, unreadable
success and possibly submitted transport loss remain **safety-only blocking**.
Retry-After is timing, not proof of non-creation. No verified gate/429 discriminator,
lookup or idempotency API has been invented. A crash before durable returned URN
needs reconciliation; the original key/name survives. Manual recovery requires
stopped processes, actual backend/public-key verification, restoring the same
identity with `PersistIdentity`, then `ArchivePhoneHomeAttempt` preserving evidence.
There is no reconciliation CLI; no power-cut guarantee or old-evidence cleanup.

## Validation and eleven-scenario checklist

All counts below are **fake supervisor RPCs**, never OS process counts. RC and AAA
counts come from real clients against loopback endpoints; enrollment uses the real
Go path against a fake backend. `TestHTTPStack*` logs status/reason, both GET counts,
Start/POST/registration counts and both retry timestamps for each scenario.
Injected-reader tests in PR 2 are identified separately, not HTTP discovery proof.

1. **Off/closed RC — implemented/tested.** Fresh and reusable identities with
   false/unknown/error/stale/absent/local-off/endpoint mismatch: 0 AAA, 0 Start,
   0 POST; RC GETs 0 or 1 depending on the fixture. Different RC key: RC=2,
   AAA/Start/POST=0. A previously obtained grant cannot reopen the gate: RC=2,
   AAA remains 1 (zero new requests), Start/POST=0. Saved identity is preserved.
2. **RC then scope grant — implemented/tested.** Final RC=6, AAA=2, Start=1,
   POST=1, registration=1; `enrolled/identity_confirmed_not_action_readiness`.
   No Start while scope is missing; AAA demand releases after persistence and
   RC continues. Discovery/enrollment retry fields are empty after completion.
3. **Quota/auth/scope rejection — verified cases recover.** Each loopback case
   ends RC=9, AAA=5, Start=2, POST=2, registration=1. Multiple positive AAA checks
   cannot shorten the saved five-minute first cooldown across restart/RC loss.
   Bare OPMS gates remain **safe-blocked-only**, not automatic recovery coverage.
4. **429 — GET recovery, POST safety-only.** Delta/date/absent/malformed GET hints
   each end RC=7, AAA=3, Start=1, POST=1, registration=1 after a following 503.
   No request precedes the saved deadline. Huge-hint overflow is separately tested.
   POST 429 remains `throttled_unverified`, RC=5, AAA=4, Start/POST=1, registration=0
   across restarts. OPMS unit tests cover POST hint parsing; no replay contract.
5. **Transient failures — bounded recovery/safety.** RC outage closes the gate;
   AAA tests reach the 15-minute backoff cap and preserve deadlines on reopen.
   Actual refused enrollment TCP connection proves `not_submitted`; restoration
   ends RC=2, AAA=2, Start=2, POST/registration=1. Possibly committed failures block.
6. **Auth/gate race — implemented/tested at layered boundaries.** Real HTTP
   POST auth/scope cases use new AAA observations after durable cooldown (case 3).
   RC-side dispatch recheck yields 0 AAA when RC closes before RoundTrip. Canceled
   or deliberately late transport results cannot grant after close/reopen,
   expiry, key generation change, demand release, disablement or shutdown.
   PAR dispatch recheck is injected-reader coverage, not real-process evidence.
7. **Bad input/unknown denial — blocked safely.** `invalid_request` and
   `forbidden_unknown`: RC=5, AAA=4, Start/POST=1, registration=0 across restarts.
   Unknown denial requires reconciliation, not a missing-scope label. Automatic
   unchanged-input recovery is intentionally absent; live config rotation unsupported.
8. **Commit/lost response — safe-blocked plus manual path tested.** RC=5, AAA=4,
   Start/POST/registration=1; `response_ambiguous`, reconciliation required.
   Pending identity bytes remain stable through restart. Test checks the original
   public key against the recorded registration and manually restores/archives
   before restart: still one POST. This does not simulate a new recovery endpoint.
9. **Persistence failure — storage recovery tested.** RC=6, AAA=2, Start=2,
   POST/registration=1; new helper waits while RC is closed, then recovers without
   a scope grant. Original component tests cover IPC/outcome-write failures and
   retained credential holders; injected controller tests preserve holders on RC
   loss. Actual holder RSS and the pre-durable-URN crash window remain unmeasured.
10. **Already enrolled — implemented/tested.** RC=6, AAA=0, Start=1, POST=0;
    same saved identity. Closed RC prevents restart; reopen starts once; later
    RC loss does not Stop the healthy process. This is not revocation enforcement.
11. **Concurrency/cancellation — layered tests pass.** Overlapping full-stack
    ticks end RC=1, AAA=1, Start/POST/registration=1. Journal tests reject stale
    writers; controller restart preserves cooldown. RC worker and component
    lifecycle tests cancel in-flight/waiting work. Post-submission ambiguity is
    covered by transport/lost-response safety tests, not real process-kill tests.

### Reproduce without live credentials (macOS or Linux)

From this stack's top worktree, run the controlled-clock loopback scenario suite;
no host-service changes or real enrollment are involved:

```sh
dda inv test --targets=./pkg/privateactionrunner/phonehome,./pkg/config/remote/service --bazel-args='--jobs=4 --test_output=all --test_timeout=120'
```

Full selected validation:

```sh
dda inv test --targets=./pkg/config/remote/service,./comp/remote-config/rcservice/impl,./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment,./pkg/privateactionrunner/opms,./comp/privateactionrunner/impl,./comp/privateactionrunner/phonehome/impl,./comp/privateactionrunner/status/statusimpl,./cmd/agent/subcommands/run --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
dda inv test --targets=./pkg/config/remote/service,./comp/remote-config/rcservice/impl,./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment,./pkg/privateactionrunner/opms,./comp/privateactionrunner/impl,./comp/privateactionrunner/phonehome/impl --race --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
```

Validated on macOS arm64: PR 1 six affected test targets plus two race targets;
PR 2 ten targets plus four race targets; PR 3 eleven targets plus eight race
targets. Logs: `/tmp/par-stack-pr{1,2,3}-*.log`; scenario evidence:
`/tmp/par-stack-pr3-scenario-evidence.log`. Earlier failed fixture/build attempts
are retained, not counted as passes. Linux execution, real process count, RSS,
wall-clock request cadence and grant-to-enrollment latency are **not measured**.

Production follow-ups: packaging/permissions, config parity and rotation,
backend recovery contracts, pending-evidence cleanup and crash durability,
revocation, AAA capacity, other platforms/flavors, rollout and real Linux QA.
Draft stack: [#58014](https://github.com/DataDog/datadog-agent/pull/58014) →
[#58015](https://github.com/DataDog/datadog-agent/pull/58015) →
[#58016](https://github.com/DataDog/datadog-agent/pull/58016).
Release notes and real Linux QA remain before merge-readiness. Local test results
do not establish the outcome of remote CI.
