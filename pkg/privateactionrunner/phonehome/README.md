# PAR phone-home — stack PR 2

This opt-in core component consumes RC status; it does not perform discovery HTTP.
PR 1 exposes the last successful RC `Enabled` / `Authorized` response, or `nil`
before the first success. Failed RC polls retain that response. Consumers use it
directly: no RC expiry, generations, provenance hashes or synthetic failure states.
A prior positive can therefore remain usable during an RC outage. A subsequent
successful negative closes the gate. Ordinary RC polling is unchanged.

PR 2 has no PAR-scope producer. Even with both RC booleans true, production fresh
enrollment waits for PR 3's PAR-owned AAA discovery in core. Scope observations
and discovery demand live under PAR; RC exposes only `GetOrgStatus()` and knows
nothing about PAR. Positive scope observations here are injected test data.

## Owners and gates

- `comp/privateactionrunner/phonehome/{def,impl,fx}` owns background lifecycle in
  core. Startup does not wait for hostname, RC, AAA or enrollment. Shutdown cancels
  work and releases demand.
- `pkg/privateactionrunner/phonehome` reads the optional RC service and controls
  registered supervisor services. Missing/local-off RC, no observation, either
  false boolean or unsupported config keeps new starts waiting.
- Every Start rechecks RC, including stopped enrolled PARs and new storage-recovery
  helpers. Saved identity bypasses AAA, not RC. Already-running healthy PARs and
  credential holders are not stopped just because RC becomes false.
- The AAA observation is only a reason, check time and next-check time. A grant
  expires after two minutes. POST auth/scope rejection still requires a check newer
  than the rejected attempt. Reading status/demand does not force a backend request.
- Go remains responsible for enrollment POST, keys and protected persistence.
  A PID is not confirmation: release demand after durable enrollment acknowledgement.

## POC configuration

Explicit `DD_PAR_PHONE_HOME_POC=true` in core/executor; locally enabled PAR;
non-containerized, non-FIPS Linux node Agent in split mode. Core and executor must
share config, resolved secrets, environment, hostname, user and an absolute identity
path. Extra PAR config and OPMS headers are unsupported. Different RC API keys,
legacy RC credentials and mismatched sites are rejected rather than treated as
PAR authorization. Live rotation remains unsupported; reader key/endpoints are
fixed for the lifetime of the component. Loopback overrides are only for the
explicit internal mock mode. On Windows the opt-in is ignored, preserving normal
PAR enrollment; POSIX journal/persistence tests run only on non-Windows platforms.

Use absolute `DD_PM_SOCKET_PATH`. Register `datadog-agent-par-control` and
`datadog-agent-action-executor` with `auto_start: false`, `restart: never`; include
`env: {DD_PAR_PHONE_HOME_POC: "true"}` for the executor. Core uses Describe/Start/Stop,
never Create, shell launch or policy rewriting. Examples in `testdata/processes.d`
do not change shipping package, Docker, Helm or Operator defaults.

## Recovery retained from the original POC

The original reference is `c2ed73ae401` / draft #57623. The enrollment journal,
exclusive POST claim, protected pending identity and retained-helper tests are
reused here. This adaptation changes the RC contract, not POST replay safety.

Verified quota/auth/scope no-creation rejections use durable 5/10/20/30-minute bases
plus 0–20% jitter, with any later Retry-After as a lower bound. RC/AAA success cannot
shorten them. Storage-only recovery uses 1/2/4/8/15-minute bases plus jitter, reusing
the same key, returned URN and attempt, without a second POST.

Pending credentials are mode 0600, separate from the credential-free 0700 journal.
If credentials or the recovery outcome cannot be safely saved, retain the Go
helper and retry local storage every minute. Its bootstrap snapshot is unavailable,
not falsely disabled. Do not discard the only key to reduce RSS. Go resolution/POST
and Rust bootstrap retain their existing 45/30/120-second limits.

Bare OPMS gates, unstructured auth, POST 429, 5xx, unreadable success and possibly
submitted transport loss remain safety-only blocking. Retry-After does not prove
non-creation. Lost responses need manual backend/public-key verification, same-key
identity restoration and evidence-preserving journal archival; no invented lookup
or idempotency API. The crash window before durable returned URN is still open.

## Validation

Controller tests inject RC/scope state and a supervisor fake; real enrollment and
persistence use a loopback POST endpoint which rejects discovery GETs. Reader tests
cover nil/all boolean combinations, last-success retention/no RC expiry, independent
AAA expiry, absence/local disablement, key overrides and frozen endpoints.
Enrollment/recovery, concurrency, cancellation and holder regressions are retained.

```sh
dda inv test --targets=./pkg/config/remote/service,./comp/remote-config/rcservice/impl,./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl,./cmd/agent/subcommands/run --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
```

No live endpoints or host services are needed. Real RC/AAA HTTP integration belongs
to PR 3. Linux process counts, RSS and wall-clock latency are not established by
these tests. Production follow-ups remain packaging, config rotation, backend
recovery contracts, crash durability, revocation, capacity and other platforms.
