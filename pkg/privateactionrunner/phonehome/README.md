# RC-gated PAR phone-home — stack PR 2

This slice depends on PR 1's typed RC org/key observations. It intentionally has
**no AAA discovery producer**: production fresh enrollment waits for PAR scope,
even when RC reports enabled/authorized. PR 3 supplies that producer on the RC
side. This package has no discovery HTTP client, parser or fallback poller.

The original direct-AAA POC remains on `jules.macret/par-phone-home-poc` at
`c2ed73ae401` / draft #57623. Its old RC-independent startup policy is superseded.
Existing enrollment safety code and tests are reused, not a new workflow engine.

## Component and ownership

- `comp/privateactionrunner/phonehome/{def,impl,fx}` owns background lifecycle
  in core. Startup never waits for RC, AAA, enrollment or hostname resolution.
  Shutdown cancels work and releases discovery demand.
- `pkg/privateactionrunner/phonehome` reads typed observations from the optional
  RC service and controls only registered process-manager Start/Stop operations.
- RC's existing `/api/v0.1/status` poll supplies independent org enablement and
  key authorization. Its interval/diagnostics are unchanged. PR 1 bounds freshness
  to two configured intervals, capped at five minutes, and fences credential
  changes, failures and stale gaps with a generation.
- PR 2's `GetPARScope()` always returns unchecked. `SetPARDiscoveryDemand` records
  interest only; it cannot issue requests or bypass a future producer's backoff.
- Go executor code owns POST/key generation/protected persistence. A service PID,
  RC permission or scope observation is not confirmed enrollment/action readiness.

## Prerequisites

This is still explicitly opt-in (`DD_PAR_PHONE_HOME_POC=true` in core/executor),
non-FIPS, non-containerized Linux node Agent, split mode, with matching config,
resolved secrets, hostname, user, local filesystem and absolute identity path.
No extra PAR config, OPMS extra headers or live credential/endpoint rotation.
The RC observation must match the enrollment key and supported RC/site endpoint;
custom mismatched endpoints fail closed. Loopback RC/PAR endpoints are allowed
only in the explicitly configured internal mock mode.

Before **every** new service Start, including a stopped enrolled PAR or a new
storage-recovery helper, require fresh matching RC `enabled && authorized`.
False, unchecked, fetch failure, stale, absent/local-off or mismatched RC blocks
new starts. Do not enable RC implicitly. Existing RC polling can observe a later
org/key grant without restarting core.

A reusable identity/storage-only recovery does not require a new PAR scope
check. Any new creation attempt does. In this slice only injected test state can
provide that grant. No production fresh enrollment or enrollment POST is possible
with the RC-only producer.

The controller rereads status at Start dispatch. If a reservation exists but the
gate closes before any Start RPC, record a proven `not_submitted` outcome rather
than leaving an ambiguous, permanently blocked reservation. A lost Start response
is still ambiguous. Unsupported live config changes release demand and wait for
matching joint restart.

**Not revocation:** a running healthy PAR or credential-retention helper is not
stopped on RC readiness loss. A holder continues safe local storage work; a new
helper waits for RC. Pending credentials/claims are never discarded to reduce RSS.

## Enrollment recovery retained from the original POC

Core owns durable enrollment deadlines independently of discovery. Verified
no-creation quota/auth/scope rejection retries use 5/10/20/30-minute bases plus
0–20% jitter; a later Retry-After is a lower bound. Scope rereads and RC reopening
cannot shorten the saved deadline. An auth rejection also saves a revalidation
floor; a cached positive predating the rejection is not a new check.

Protected `<identity>.pending-<attempt-ID>` files save key/name before POST and
returned URN before active publication. The separate 0700 outcome journal has
exclusive POST claims and bounded categories, no credentials. Atomic publication
and file/directory fsync precede success acknowledgement. A visible file alone
is insufficient to declare enrollment and release demand.

Storage-only recovery reuses the same attempt/key/URN with 1/2/4/8/15-minute bases
plus jitter and no second POST. The helper stays alive until both its credentials
and recovery outcome are durable, even if IPC/outcome storage is broken; its
snapshot is unavailable, never falsely disabled. It retries storage every minute,
then can exit so core schedules publication. Rust retains its 120-second budget;
Go resolution/POST retain their 45/30-second limits.

Unknown 403, unstructured auth, enrollment 429, 5xx, unreadable success and possibly
submitted transport failures remain **safety-only blocking**. Retry-After is
retained but does not prove non-creation. No verified backend lookup/idempotency
contract or gate/429 discriminator has been added. A crash before durable URN
remains reconciliation-only, with the original key preserved. Deliberate manual
recovery requires stopped processes, actual backend/public-key verification,
restoration of the same identity, and evidence-preserving journal archival.

## Isolated supervision

Use explicit absolute `DD_PM_SOCKET_PATH` and register
`datadog-agent-par-control` / `datadog-agent-action-executor` with both
`auto_start: false` and `restart: never`. Executor registration explicitly sets
`env: {DD_PAR_PHONE_HOME_POC: "true"}`. Core refuses conflicting policies and never
creates processes, shells out, or rewrites supervisor policy. Shipping package,
Docker, Helm and Operator templates remain unchanged. `testdata/processes.d`
contains wiring examples, not a complete real-process harness.

## Validation and scope of evidence

Controller tests inject RC/PAR observations and use a supervisor fake. Where
appropriate they invoke the real Go enrollment/persistence client against a
loopback POST endpoint that rejects any discovery GET. This is **injected-state
coverage**, not proof of RC-owned HTTP discovery; that belongs in PR 3.

Tests cover closed RC states with saved identity/cached grant, production unknown
scope, reopening, dispatch races, cooldown/auth revalidation across restart,
ambiguous outcomes, storage-only recovery waiting for RC, helper retention,
concurrent ticks, disablement, cancellation and supervisor policy. Existing
Go enrollment/OPMS/component regressions stay with this slice. The RC reader's
four boolean combinations, staleness, missing service and mismatched provenance
are tested separately. No Linux process/RSS/latency measurements are claimed.

```sh
dda inv test --targets=./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment,./pkg/privateactionrunner/opms,./comp/privateactionrunner/impl,./comp/privateactionrunner/phonehome/impl,./comp/privateactionrunner/status/statusimpl,./pkg/config/remote/service,./cmd/agent/subcommands/run --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
dda inv test --targets=./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment,./comp/privateactionrunner/phonehome/impl --race --bazel-args='--jobs=4 --test_output=errors --test_timeout=120'
```

Validated on macOS arm64: 10 focused/core test targets passed; 4 focused race
targets passed. Component/Fx/CODEOWNERS lint passed. The first race run found
unsynchronized fake-backend counters in the lost-response test; a handler-completion
channel fixed that fixture, and the race rerun passed. Logs are under
`/tmp/par-stack-pr2-{full,race-final,component-lint-final}.log`.

No live keys/backend calls, host-service changes or deployments are needed. Full
RC HTTP → AAA HTTP → controller coverage and the complete eleven-scenario report
remain PR 3 work. Production follow-ups still include packaging, configuration
parity/rotation, supervisor permissions/lifecycle, backend recovery contracts,
retention cleanup, power-cut testing, revocation, AAA capacity and other platforms.
