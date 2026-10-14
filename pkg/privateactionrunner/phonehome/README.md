# PAR phone-home — RC-first opt-in POC

Draft stack: [#58014](https://github.com/DataDog/datadog-agent/pull/58014) →
[#58015](https://github.com/DataDog/datadog-agent/pull/58015) →
[#58016](https://github.com/DataDog/datadog-agent/pull/58016).
The original POC #57623 is preserved. Shipping defaults are unchanged.

## Simple RC contract

PR 1 returns a copy of the last successfully fetched `OrgStatusResponse`, or `nil`
before the first success. **Failed polls retain that response; it does not expire.**
Consumers read `Enabled && Authorized` directly. There are no RC freshness states,
generations, credential-binding hashes or synthetic fetch-failure observations.
A prior positive can remain usable during an RC outage. A successful negative,
missing observation or absent/local-off RC blocks new AAA requests and PAR starts.
This intentionally replaces the earlier POC's expiring RC-observation design.

PR 2 adds the core PAR component. It reads status and owns supervisor launch and
enrollment reconciliation, with no HTTP discovery client or fallback. Its scope
producer is intentionally unchecked: production fresh enrollment waits for PR 3.

PR 3 adds the opt-in AAA worker to core's RC service, not PAR's RC client. It uses
the same RC booleans and publishes only the AAA reason, check time and next-check
time. Go PAR still owns POST, keys and persistence. Both background lifecycles
cancel on shutdown and do not delay core startup waiting for permission.

## Requests, starts and retries

- Every AAA HTTP dispatch and PAR Start checks the RC booleans again. Discovery
  also needs explicit demand from the opted-in, unenrolled controller.
- AAA calls `GET /api/v2/validate` with the enrollment API key only. HTTP 200 is
  insufficient: require `valid: true` plus `private_action_runner_enroll`.
  `remote_config_read` is not PAR enrollment permission. The normal Agent transport
  handles proxy/TLS; redirects are refused and failures expose bounded reasons,
  not raw bodies or transport errors. JSON is bounded and fully parsed.
- AAA grants expire after two minutes, independently of RC. An observed closed
  gate invalidates the cached AAA grant. Demand release/shutdown cancels requests;
  the in-flight watch also cancels when it observes false RC or unsupported config.
  Canceled responses cannot publish a grant, even after reopening. There is no
  promise to detect a false/true transition occurring entirely between reads.
- Startup jitter: 0–60s. Normal/missing-scope cadence: 60–72s. Transient failure
  bases: 1/2/4/8/15m, plus 0–20% jitter. Auth/other HTTP failures: 15m plus jitter.
  GET 429 respects Retry-After delta/date, minimum/fallback one minute. Huge hints
  cannot overflow when jitter is added. Deadlines survive pause/reopen and demand
  changes, not a complete core restart. Reads cannot force a request.
- Enrollment cooldown is separate and durable: verified quota/auth/scope rejection
  bases 5/10/20/30m plus jitter, subject to any later Retry-After. Successful RC/AAA
  checks cannot shorten it. A POST auth/scope failure requires an AAA check newer
  than the rejected attempt; rereading the old grant is not revalidation.
- Release AAA demand after durable enrollment acknowledgement, not a PID. Ordinary
  RC polling continues. Reusable identities and storage-only recovery need RC but
  no new AAA scope check. Running healthy PARs and credential holders are not
  stopped merely because RC becomes false. This is not a revocation policy.

## Deployment boundary

Explicit `DD_PAR_PHONE_HOME_POC=true` in core and executor, locally enabled PAR,
non-containerized/non-FIPS Linux node Agent, split mode. Share main config,
resolved secrets, environment, hostname, user/filesystem and an absolute identity
path. Extra PAR config, OPMS headers and live key/site rotation are unsupported.
Different RC API keys, legacy RC credentials and mismatched sites are rejected;
startup key/endpoints are fixed for each component lifetime. Paired loopback
endpoints are allowed only in explicit internal mock mode.

Set absolute `DD_PM_SOCKET_PATH`. Register `datadog-agent-par-control` and
`datadog-agent-action-executor` with `auto_start: false`, `restart: never`; executor
registration includes `env: {DD_PAR_PHONE_HOME_POC: "true"}`. Core only uses
Describe/Start/Stop, not Create, shell launch or policy rewriting. Examples in
`testdata/processes.d` do not change package/Docker/Helm/Operator defaults.

## Enrollment safety is unchanged

The original journal, exclusive POST claim, protected pending identity and helper
retention remain. A 0600 pending file holds key/name before POST and the returned
URN before active publication. A separate 0700 journal holds bounded outcomes and
deadlines, not credentials. A visible identity alone is not acknowledgement.

Storage-only recovery reuses the same key/URN/attempt, with 1/2/4/8/15m bases plus
jitter and no second POST. If credentials or the outcome cannot be saved, retain
the Go helper, retry local storage every minute and serve an unavailable bootstrap
snapshot, never falsely disabled. Do not discard the only key to reduce RSS.
Existing Go resolution/POST and Rust bootstrap limits remain 45/30/120s.

Bare OPMS gates, unstructured auth, POST 429, 5xx, unreadable success and possibly
submitted transport loss remain **safety-only blocking**. Retry-After does not prove
non-creation. Manual lost-response recovery needs stopped processes, actual backend/
public-key verification, same-key identity restoration and journal archival. No
lookup/idempotency endpoint is invented. The pre-durable-URN crash window remains.

## Acceptance coverage

`TestHTTPStack*` uses real RC/AAA/enrollment clients against loopback servers and a
fake supervisor. It logs RC GETs, AAA GETs, Start/POST/registration counts, reasons
and separate retry times. Injected-reader and component tests cover additional
boundaries; none establish real OS process counts or RSS.

1. **Off/closed RC:** nil, either false flag, absent/local-off service, config
   mismatch and opt-out produce zero new AAA/Start/POST, including saved identities.
   An initial failed poll leaves nil; a failure after success retains the last
   response and allows work. Cached RC age alone no longer blocks startup.
2. **RC then scope grant:** no AAA until both RC booleans; no fresh Start without
   PAR scope. One persisted registration releases demand; ordinary RC polls continue.
3. **Quota/auth/scope:** verified rejections recover after durable cooldown, despite
   repeated positive scope checks and controller restart. Bare gates stay blocked.
4. **429:** GET delta/date/missing/malformed hints retain deadlines across toggles;
   overflow is tested. POST hint parsing is tested separately; unverified rejection
   remains blocked, not automatic recovery.
5. **Transient failure:** AAA backoff is bounded; proven refused enrollment TCP
   submission recovers. Ambiguous POST failures do not replay. RC fetch errors
   retain the last successful booleans, as specified by PR 1.
6. **Auth/gate race:** newer AAA revalidation after auth/scope rejection; request
   and Start dispatch checks; canceled late responses cannot grant. No RC generations.
7. **Bad input/unknown denial:** distinct blocked categories, no repeated unchanged
   launch. Config rotation is unsupported; deliberate reconciliation remains needed.
8. **Commit/lost response:** one registration, stable pending key across restart;
   manual public-key verification, restoration and archival tested. No fake recovery API.
9. **Persistence failure:** same-key storage recovery without a second POST; new
   helpers wait for RC, running holders survive RC loss. IPC/outcome failures are
   component-tested; actual crash/power-cut durability remains unmeasured.
10. **Already enrolled:** same identity, no AAA/registration; stopped PAR requires
    RC booleans. Later RC loss does not Stop a healthy process.
11. **Concurrency/cancellation:** overlapping ticks cannot duplicate registration;
    late journal writes are rejected; cooldown survives restart; shutdown cancels
    waiting/in-flight work. Post-submission loss remains ambiguous.

```sh
dda inv test --targets=./pkg/config/remote/service,./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
dda inv test --targets=./pkg/config/remote/service,./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl --race --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
```

These fixtures also provide a Linux-local reproduction without live credentials
or service changes. This session validates on macOS; real Linux process/RSS,
wall-clock cadence and grant latency remain unmeasured. Production follow-ups:
packaging, config rotation, backend recovery contracts, crash durability, cleanup,
revocation, AAA capacity and other platforms. Release notes remain pending by request;
local tests do not establish remote CI success.
