# RC- and AAA-gated PAR startup POC

The core Agent combines an RC startup gate with AAA scope discovery through
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
existing limitations on ambiguous network failures. The startup gate adds no enrollment POST
retry, journal, idempotency promise or credential-recovery policy. An operator restart can
re-arm the gate: reconcile any possibly committed enrollment before manually retrying. The
provided supervisor and core QA unit must not independently restart failed attempts.

Old POC `.phone-home` attempt journals block new startup rather than silently discarding
pending credentials. Preserve/reconcile old experimental state before migrating test VMs.

Internal `par_phone_home` expvar reports waiting/blocked/handoff and a bounded reason.
When the POC is enabled, `datadog-agent status` also reports the startup-gate state,
reason and next check in JSON, text and HTML.

## AAA discovery

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

## COAT diagnostics

The opted-in core component records `par_phone_home.state` (a gauge) and
`par_phone_home.transitions` (a counter), with only `state` and `reason` labels.
The gauge is one-hot: the previous pair is set to zero. The counter counts entries,
including the initial observation and re-entry, not repeated polls or next-check changes.
These describe startup gating only; `handed_off` is not enrollment or runtime health.

The default COAT profile collects after 30 seconds and every 15 minutes while Agent
telemetry is enabled. It sends the current gauge snapshot and counter increments since
collection, not a time-averaged gauge or an ordered transition log. Zero gauge rows are
excluded. Intermediate states can disappear from the snapshot but remain in the counter.
The terminal handoff snapshot remains available after the startup worker exits.

COAT adds `emitter` and existing host/OS metadata; version and org attribution use its
existing transport/backend path. No new host, org, credential, URL or raw-error labels
are added. Diagnostics use the existing Instrumentation Telemetry intake and dedicated
COAT org, not customer DogStatsD metrics or a new org2 destination. They are best-effort:
collection advances counter baselines before successful delivery and there is no new
shutdown flush, retry queue or network client here. Query the latest fresh snapshot per
host rather than summing repeated reports as distinct hosts. Custom COAT profiles must
select these metrics explicitly; the new selection is in the default profiles only.

## Validation

`dda inv test --targets=./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl,./comp/privateactionrunner/impl`

Tests retain RC-only source coverage and add real loopback RC/AAA requests for all eight
permission combinations, each with and without an existing identity file. They cover delayed
grants, dispatch/launch gate loss, expired/late grants, cancellation, backoff/Retry-After,
strict parsing and redirects. Supervisor calls are mocked; these are not Linux process or
live enrollment tests.
