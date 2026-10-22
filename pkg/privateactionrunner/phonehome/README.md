# RC-gated PAR startup POC

PR 2 adds only a core-owned startup gate. RC's last successful `Enabled && Authorized`
permits starting PAR, including a fresh installation. It performs no discovery HTTP
and does not change Go enrollment, persistence, retries or Rust bootstrap.

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

RC nil/false waits without starting PAR. A later positive RC response opens the gate
without a core restart. Failed RC polls retain the last successful response indefinitely;
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

## Validation

`dda inv test --targets=./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl,./comp/privateactionrunner/impl`

Tests cover the production RC-only source with all four flag combinations, delayed grants,
FQDN/config mismatch, cancellation, no duplicate Start after an uncertain result, leaving
existing processes untouched, legacy-journal migration blocking, and the unchanged standalone
enrollment path. Mock supervisor tests do not prove Linux process startup or live enrollment.
