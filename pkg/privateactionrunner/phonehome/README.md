# RC- and AAA-gated PAR startup POC

PR 2 adds an independently useful RC-only startup gate. PR 3 composes it with AAA
`GET /api/v2/validate`: after RC's last successful `Enabled && Authorized`, require
`private_action_runner_enroll` before **every new PAR start**, including a stopped
runner with a saved identity. Neither gate changes Go enrollment/persistence/retries
or Rust bootstrap. Running children and potential credential holders are left alone.

This is an explicitly opted-in, specially configured Linux node-Agent POC. **Ordinary
packages are not gated:** shipping `par-control` registrations still auto-start.
Use a separate test supervisor, not an overlay on a running installation.

## Configuration

- Core: `DD_PAR_PHONE_HOME_POC=true`, PAR enabled, split mode enabled, local RC enabled.
- Non-containerized, non-FIPS Linux core Agent; matching core/executor config, API key,
  site, proxy, secrets and user. RC-specific credentials/unsupported endpoint overrides
  and extra PAR config files are not supported. Live credential/endpoint changes need
  a coordinated restart.
- Absolute `private_action_runner.identity_file_path` and `DD_PM_SOCKET_PATH` in core.
- Both child registrations must have `auto_start: false`, `restart: never`.
  Explicitly pass `DD_PM_SOCKET_PATH` to children; supervisor environment is not
  automatically inherited. Example registrations are under `testdata/processes.d`.
- Set `DD_PAR_POC_BIN` to the packaged binary directory and `DD_PAR_POC_DIR` to an
  isolated configuration/IPC directory, with `DD_PM_SOCKET_PATH=$DD_PAR_POC_DIR/procmgr.sock`.
  Configure the same executor socket and IPC certificate in core/PAR config.
- For fresh enrollment, enable ordinary PAR API-key self-enrollment. Existing identities
  are handled by the existing PAR bootstrap, not by the gate.

## Lifecycle and limits

RC nil/false waits without starting PAR or calling AAA. A later positive RC response
allows AAA discovery without a core restart; a PAR-scope grant then permits startup. Failed RC polls retain the last successful response indefinitely;
this POC adds no freshness framework. The gate checks RC again immediately before Start.

There is **at most one Start RPC per core process lifetime**. Acknowledgement means only
that startup was handed to the supervisor, not that enrollment or action readiness succeeded.
After a successful or ambiguous Start, this controller does not retry or monitor enrollment.
A failed/exited child requires operator review; already-active children are left untouched.
It never stops an action-serving PAR or a process that might hold credentials.

The existing enrollment implementation retains its own retry/persistence behavior and its
existing limitations on ambiguous network failures. This PR adds no new enrollment POST
retry, journal, idempotency promise or credential-recovery policy. An operator restart can
re-arm the gate: reconcile any possibly committed enrollment before manually retrying. The
provided supervisor and core QA unit must not independently restart failed attempts.

Old POC `.phone-home` attempt journals block new startup rather than silently discarding
pending credentials. Preserve/reconcile old experimental state before migrating test VMs.
The previous recovery experiment is retained in backup refs, outside this scoped stack.

Internal `par_phone_home` expvar reports waiting/blocked/handoff and a bounded reason.
No new `datadog-agent status` fields are added here; those belong to PR 4.

## AAA discovery (PR 3)

The PAR-owned core worker uses the same RC source as startup; it does not duplicate
RC freshness or identity rules. Requests use only `DD-API-KEY`, a 10-second timeout,
Agent proxy/TLS transport, bounded JSON parsing and no redirects. Polling has jitter,
backoff and Retry-After handling; cancellation and gate rechecks prevent late grants.
AAA grants expire after two minutes, independently of RC's retained-last-success policy.
Nil discovery fails closed, rather than restoring the RC-only behavior accidentally.

Discovery ends when core hands startup to the supervisor, not when enrollment is
confirmed. The established Go enrollment path owns everything after that handoff.
A subsequent deliberate core restart with stopped children requires fresh AAA approval,
even with a saved identity. No post-enrollment revocation or automatic relaunch is added.

## Validation

`dda inv test --targets=./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl,./comp/privateactionrunner/impl`

Tests retain RC-only source coverage and add real loopback RC/AAA requests for all eight
permission combinations, each with and without an existing identity file. They cover delayed
grants, dispatch/launch gate loss, expired/late grants, cancellation, backoff/Retry-After,
strict parsing and redirects. Supervisor calls are mocked; these are not Linux process or
live enrollment tests.
