# Core-owned PAR phone-home POC

Opt-in experiment, not a rollout/default-on change. Extends the original POC
`35f0026b39d` for the Oct 6 enrollment-recovery brief. Shipping templates, backend
APIs, RC authorization, and Rust bootstrap deadlines are unchanged. Enrollment
confirmation is **not action readiness**.

## Deployment and ownership contracts

- One non-containerized Linux node Agent, non-FIPS, split mode, local
  `dd-procmgrd`. Other platforms/flavors, Kubernetes, and arbitrary PAR-local
  configuration are outside this POC.
- Set `DD_PAR_PHONE_HOME_POC=true` in core and executor; also enable PAR and
  `private_action_runner.split_enabled`. Opt-out preserves existing behavior.
  Local disablement stops controller activity, not an already-running PAR.
- Core and executor must use matching files, environment, resolved secrets,
  hostname, user, local filesystem, and explicit absolute `identity_file_path`.
  No extra PAR config, OPMS extra headers, per-process credentials/proxies, Fleet
  overrides, or live rotation. Core neither loads nor overwrites PAR-local config.
  A protected digest binds enrollment-critical settings/credential to the
  attempt; this is not proof of arbitrary config parity. Restart both after
  configuration changes. Ambiguous attempts still require reconciliation.
- Set `DD_PM_SOCKET_PATH` to the isolated supervisor's absolute socket path.
  Register `datadog-agent-par-control` and `datadog-agent-action-executor`, both
  `auto_start: false`, `restart: never`. The executor definition must explicitly
  set `env: {DD_PAR_PHONE_HOME_POC: "true"}`. Core checks policy and uses only
  Describe/Start/Stop of these registrations. Stop is restricted to failed,
  owned startup attempts, not a healthy enrolled PAR. No Create, shell spawning,
  policy rewriting, or parallel systemd/s6/kubelet/monolithic launch authority.
- Existing identities use the shared reader and hostname/API-key-hash reuse
  rules. Legacy app-key identities without hashes remain usable. Initial startup
  and existing re-enrollment bypass a new scope gate. A verified rejection can
  subsequently require current validation before retrying.

## Discovery and retry ownership

Core calls API-key-only `GET /api/v2/validate`; both `valid: true` and
`private_action_runner_enroll` are required. RC scope is irrelevant. The Agent's
transport preserves TLS/proxy behavior; redirects are refused. API keys, response
bodies and raw network errors are not logged or included in status.

Concrete POC schedules:

- Initial discovery jitter: 0–60 seconds. Missing scope/normal checks: 60–72 seconds.
- Discovery transport/5xx/malformed response: 1, 2, 4, 8, then 15 minutes, plus
  up to 12 seconds jitter. Invalid credentials/other HTTP failures: 15 minutes
  plus jitter. Discovery 403 is not assumed to mean missing scope.
- Discovery 429: at least Retry-After (delta-seconds or HTTP date); missing,
  malformed, negative or past hints fall back to one minute, plus jitter.
- Verified no-creation enrollment rejection: 5, 10, 20, then 30-minute base,
  plus 0–20% jitter. A later Retry-After, including on a verified quota/auth
  rejection, is a lower bound. Deadlines and attempt
  numbers survive restart; successful scope checks do not reset them.
- Durable pending-identity publication: 1, 2, 4, 8, then 15-minute base, plus
  0–20% jitter. This reuses the same attempt/key and performs **no creation POST**.
- In-memory credential holder: storage-only retries every minute until the
  returned identity can be saved. It is an explicit resource-retention exception.

Core alone reserves attempts and schedules retries. Go performs key generation,
one claimed POST, and persistence; the supervisor cannot restart independently.
Rejected startup processes are stopped before another launch is authorized.
There is no minute-scale POST retry loop. Go identity resolution is bounded to
45 seconds, its HTTP POST to 30 seconds, and Rust keeps its 120-second bootstrap
budget. A persistence holder returns an unavailable config snapshot, never a
successful `split_mode=false` snapshot, and cannot execute actions.

### Verified rejection contract and remaining API gaps

Read-only source inspection used dd-source `52e8d4f992917431f8ac7b87dd5e640c31d01795`:

- `domains/actionplatform/apps/apis/opms/handler/create_on_prem_runner_api_key_only.go`
- `domains/actionplatform/apps/wf-actions-server/src/connection/conngrpc/create_on_prem_runner.go`
- `domains/app-builder/apps/shared/libs/errors/structured_error.go`
- `domains/api_platform/shared/libs/go/rapid/responder_jsonapi.go`
- `domains/experiments/shared/libs/go/experimentshttp/experimentshttp.go`

The POC requests JSON:API and recognizes only exact, single structured errors
with matching HTTP status/title: quota (403), `required scope missing` (403), and
`invalid auth context` (401) prove rejection before creation in the inspected
path. Known request-validation titles (400) block unchanged input. A name conflict
requires reconciliation; it does not identify a recoverable runner. A dial
failure proves the request was not submitted; read/write failures do not.

**Safety-only, not automatic-recovery completion:** the inspected OPMS gate
returns an empty 403 with no unique discriminator. Unstructured 401/403, unknown
4xx, 5xx, malformed/unreadable successes, transport loss after possible submission,
and enrollment 429 therefore block replay. Enrollment 429 retains Retry-After
(delta/date, five-minute fallback) but timing does not prove non-creation. There
is no verified API-key-only idempotency/lookup contract for a lost creation
response. Backend error discriminators/recovery contracts need discussion; no
new endpoint or invented fake idempotency behavior was added.

## Durable attempts and protected identity

`<identity>.phone-home/` is a mode-0700, credential-free journal. `current.json`
contains a random attempt ID, counter, start time and enrollment/recovery
schedule. Each attempt subdirectory has an exclusive synced `post` claim and a
bounded-category `outcome.json`. Stale outcome/schedule writers are rejected;
late writes cannot clear a newer generation. Old-format/unreadable journals
require reconciliation instead of being silently reset.

Credentials are separate, in mode-0600 `<identity>.pending-<attempt-ID>` files:

1. Core writes only a config binding. Go saves its generated key and unique runner
   name **before POST**. Neither raw API key nor private key enters the journal.
2. On a valid creation response, Go saves the returned URN alongside that key,
   then atomically publishes/syncs the normal identity file.
3. Only `identity_persisted` acknowledgement plus a reusable identity completes an
   owned attempt. A visible file or running PID alone does not stop discovery.
4. If publication fails, the next Go bootstrap retries storage using that same
   identity. If saving the returned URN **or its recovery outcome** fails, keep
   the helper alive with no action executor/idle exit until both can be saved.
   Durable credentials alone are insufficient: core needs the outcome to schedule
   the next storage-only retry, even when the active identity is already visible.
   Broken IPC does not tear down that holder. Without an outcome, an active
   executor past bootstrap budget is conservatively retained because it might
   hold credentials. This uncertainty is visible in status.
5. Core archives completed journal evidence and removes the completed pending
   record. Prior rejected-attempt pending files/evidence are retained in this
   POC; bounded retention/cleanup is a follow-up.

Use private local storage writable only by the test Agent user. Atomic rename and
file/directory fsync improve durability; they are not power-cut validation or a
cross-host/distributed lock. **Unsupported automatic crash window:** after remote
commit but before a complete response/URN is durably saved, a killed holder can
lose the returned URN. The already-saved private key/name and POST claim remain;
reconciliation is mandatory, never another blind creation POST.

### Deliberate manual recovery

Stop the isolated core/controller and both registered PAR processes first.
Preserve the journal and pending/active identities. An authorized administrator
must determine whether a runner was created and verify the exact public key and
org/runner identity; matching a name or receiving a duplicate-name error is not
sufficient.

- If creation is confirmed, restore the **original** private key with the verified
  URN/hostname/API-key hash using the existing identity format/persistence path.
  Do not generate another key. The lost-response regression verifies the backend's
  actual submitted public key before calling `PersistIdentity` with that key.
- If non-creation is established, fix the relevant input/backend problem first.
- Deliberately archive the stopped attempt with `ArchivePhoneHomeAttempt(cfg, ID)`
  (equivalent to renaming `<identity>.phone-home` to
  `<identity>.phone-home.reconciled-<ID>`), then restart core with matching config.
  This preserves evidence and pending credentials. Never delete the identity or
  clear an ambiguous claim simply to force another POST.

There is no standalone reconciliation CLI or automatic backend lookup in this
POC. The tested manual path uses existing Go persistence plus the archive helper;
an operational tool/UI and authorization procedure remain follow-ups.

## Reproduce locally with no live enrollment

On a Linux development checkout with the Agent toolchain (also runnable on macOS):

```sh
dda inv test --targets=./pkg/privateactionrunner/phonehome \
  --bazel-args='--jobs=4 --test_output=all --test_timeout=60'

dda inv test --targets=./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment,./pkg/privateactionrunner/opms,./comp/privateactionrunner/impl,./comp/privateactionrunner/status/statusimpl,./cmd/agent/subcommands/run \
  --bazel-args='--jobs=4 --test_output=errors --test_timeout=60'

dda inv test --targets=./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment,./pkg/privateactionrunner/opms,./comp/privateactionrunner/impl \
  --race --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
```

These use loopback backend responses, real Go enrollment/key/persistence code, a
supervisor fake, controlled controller time/jitter and a mock retention clock.
No minute-scale sleeps, real credentials, customer keys or service installs.
Controller test logs print status/reason, Start/Stop, POST/registration counters,
retry timestamps and reconciliation flags. Counts below are fake supervisor RPCs
and loopback registrations, **not measured OS process counts**.

### Acceptance checklist

1. **Off — implemented/tested (unit):** opt-out/disabled Start is a no-op: no
   discovery, Start or POST, no identity changes/deadline. Core command tests pass;
   a full Linux core daemon health test is not claimed.
2. **Unscoped → granted — implemented/tested (fake backend):** three waiting
   checks, Start/POST/registration 0/0/0; then 1/1/1 and `enrolled`. Pending launch
   keeps discovery active; persistence acknowledgement clears its deadline.
3. **Always scoped, rejection clears — partial:** verified quota automatically
   recovers, 2 Starts/2 POSTs/1 registration; no calls before the durable five-minute
   enrollment deadline despite successful GETs/restart. Repeated failures test
   5/10/20/30/30-minute bases. Supervisor auto-start/restart is rejected. Bare gate
   403 is **safe-blocked-only**, 1/1/0, `forbidden_unknown`, manual reconciliation.
4. **429 — partial:** GET delta/date/missing/malformed hints and lower-bound waiting
   are tested; successful discovery resumes after the deadline. POST has 1 Start,
   1 POST, 0 registrations in the rejection fake, `throttled_unverified`, no
   automatic next attempt; the retained Retry-After cannot authorize replay.
   Oversized/truncated 429 bodies do not discard the header. Automatic POST-429
   recovery remains missing a verified rejection discriminator.
5. **Transient failures — implemented/tested with safety boundary:** discovery
   1/2/4/8/15-minute backoff and recovery; dial failure makes 2 Starts but only 1
   actual submitted POST/registration after cooldown. Ambiguous transport/5xx
   remains blocked with the same prepared key/claim, not retried blindly.
6. **Auth race — implemented/tested for structured contract:** invalid auth context
   or missing scope overrides preflight, revalidates, respects cooldown, then
   recovers (2/2/1). Core/executor mismatch is blocked before POST. Unstructured
   401 is safety-only (`unauthorized_unverified`), not automatic recovery.
7. **Bad input/unknown denial — implemented/tested:** one failed launch/POST,
   `invalid_request` versus `forbidden_unknown`, no automatic retry deadline or
   repeated unchanged launch. Deliberate archive/restart after a known rejection
   and backend fix yields 2/2/1. Live config change requires matching joint restart.
8. **Commit then lose response — safety/manual path tested:** 1 registration,
   1 POST, 1 initial Start across repeated restarts; key/name unchanged and
   reconciliation required. Verified manual identity restoration makes a second
   Start with **no** second POST. Automatic lookup/idempotency is not implemented.
9. **Commit then persistence fails — implemented/tested (fake/in-process):** normal
   publication failure makes 2 Starts, 1 POST/registration; storage recovery reuses
   the same key/URN after a one-minute deadline. Memory-only URN retention uses
   one submitted POST, unavailable snapshot/health-not-ready, no action executor,
   then storage restoration and shutdown. Also tested with broken IPC, failed
   outcome writes (with and without active-identity write failure), missing
   acknowledgement and loss of the holder. Crash before durable URN remains a
   reconciliation-only window, not guaranteed automatic recovery.
10. **Existing identity/restart — implemented/tested:** legacy identity starts once,
    no GET/POST/new registration, no Stop on absent scope. Existing hostname/key
    re-enrollment still makes its intentional single POST without a new initial
    scope gate; it is not confused with fresh installation.
11. **Concurrency/cancellation/deadlines — implemented/tested:** 20 overlapping
    ticks produce one Start/reservation; exclusive claim permits one POST; stale
    results/schedules cannot replace a new attempt. Core cooldown survives restart.
    In-flight cancellation after submission remains ambiguous; cancellation before
    sending is retry-safe. A visible unacknowledged identity does not stop GETs.

All eleven have unit/fake coverage as qualified above, **none real Linux process
coverage**. Gate, unstructured-auth and POST-429 automatic recovery are incomplete.

## Observability, results and real-process boundary

`agent status` text/HTML/JSON and expvar `par_phone_home` distinguish state/reason,
scope checked/readiness, enrollment outcome, retry safety, reconciliation,
next discovery, next enrollment attempt, Retry-After hint and helper retention.
An unchecked scope is unknown, not known absent. Failure categories are bounded;
no credential fields or arbitrary backend messages enter this payload. `enrolled`
means identity confirmation, not RC signing-key readiness or usable actions.

Oct 6 validation on **macOS arm64**: focused/core suites passed **8 Bazel targets**;
controller/enrollment/OPMS/component race suites passed **5 targets**. Gofmt,
Gazelle diff mode, buildifier and `git diff --check` passed. Earlier fixture failures
and Bazel-server crashes after tool timeouts were resolved and rerun; they are not
reported as passing runs. A subsequent review reproduced four outcome-write
retention failures before fixing them; all 8 focused/core and 5 race targets
passed again. Latest logs: `/tmp/par-phone-home-review-tests.log` and
`/tmp/par-phone-home-review-race.log`; the pre-fix regression is in
`/tmp/par-phone-home-review-regression.log`. No real enrollment/deployment or
host-service changes.

`testdata/processes.d/` has isolated wiring examples, not a complete real-process
harness. A separately approved Linux run needs matching branch binaries, a private
`DD_PM_CONFIG_DIR`, `DD_PM_SOCKET_PATH`, `DD_PAR_POC_BIN`, `DD_PAR_POC_DIR`, shared
IPC certificate/hostname, and a loopback mock that also handles Rust OPMS traffic.
Use `DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS=true` in both Go processes for that mock.
Do not point it at production/customer keys. Example config:

```yaml
api_key: fake-test-key
dd_url: http://127.0.0.1:18080
remote_configuration:
  enabled: false
private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: true
  api_key_only_enrollment: true
  identity_file_path: /absolute/private/sandbox/identity.json
```

Linux supervisor permissions, real process count, waiting/retained-helper RSS,
wall-clock cadence and scope-grant-to-enrollment latency remain **unmeasured**.
The retained helper expressly invalidates any blanket zero-PAR-RSS failure claim.

## Production follow-ups

- Backend: stable no-creation discriminators (especially gate/429/auth), supported
  idempotency/recovery, operational reconciliation tooling, power-cut validation.
- Packaging: conditional launch ownership/upgrades, core-disabled fallback and
  post-enrollment supervision. Kubelet starts declared containers independently;
  this POC does not remove their footprint.
- Config: actual PAR extra-file/env/secret/Fleet parity, rotation, endpoint/proxy
  refresh; supervisor permissions and matching-version lifecycle validation.
- Storage: bounded cleanup of rejected-attempt evidence/pending keys and retention
  helper resource measurements; do not compromise credential recovery for RSS.
- Revocation: verify post-enrollment key/scope removal; preserve existing behavior
  rather than inventing an enforcement guarantee.
- Rollout/capacity: AAA rate approval and fleet sizing, split-default prerequisite,
  other OS/flavor/FIPS support before any wider opt-in.
