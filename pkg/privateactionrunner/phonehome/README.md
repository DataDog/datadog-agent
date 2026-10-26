# RC- and AAA-gated PAR startup POC

Core composes the RC startup gate with AAA `GET /api/v2/validate`: after RC's
last successful `Enabled && Authorized`, require `private_action_runner_enroll`
before each core-owned startup, including a stopped
runner with a saved identity. Neither gate changes Go enrollment/persistence/retries
or Rust bootstrap. Running children and potential credential holders are left alone.

The worker is enabled by `private_action_runner.enabled` on non-Windows platforms;
there is no separate phone-home opt-in. Startup remains restricted to a specially
configured Linux node-Agent POC. **Ordinary packages are not gated:** shipping
`par-control` registrations still auto-start. Without the required configuration,
the worker reports a blocked state rather than changing existing startup behavior.
Use a separate test supervisor, not an overlay on a running installation.

## Configuration

- Core: PAR enabled, split mode enabled, local RC enabled.
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

After a successful handoff or observing existing active children, core watches both
supervisor registration UUIDs once a minute. A supervisor restart recreates both
registrations; only then does core re-arm startup and require RC plus a fresh AAA grant.
Replacing only one registration waits without starting anything. Losing connectivity
alone does not re-arm the gate, and a recovered connection restores the handoff state.

There is at most one core Start request for the same observed registration lifecycle.
An ambiguous Start stops the controller and requires operator review, even if the
supervisor later restarts. Acknowledgement is not proof of enrollment or action readiness.
Ordinary child-process crashes/restarts keep their registration IDs: core does not
re-authorize, relaunch or stop them. A failed/exited child observed before handoff requires
operator review. Already-active children and potential credential holders are left alone.

The existing enrollment implementation retains its own retry/persistence behavior and its
existing limitations on ambiguous network failures. The gate adds no new enrollment POST
retry, journal, idempotency promise or credential-recovery policy. An operator restart can
re-arm the gate: reconcile any possibly committed enrollment before manually retrying. The
provided supervisor and core QA unit must not independently restart failed attempts.

Old POC `.phone-home` attempt journals block new startup rather than silently discarding
pending credentials. Preserve/reconcile old experimental state before migrating test VMs.

Internal `par_phone_home` expvar reports waiting/blocked/handoff and a bounded reason.
This controller does not add status-command presentation.

## AAA discovery

The PAR-owned core worker uses the same RC source as startup; it does not duplicate
RC freshness or identity rules. Requests use only `DD-API-KEY`, a 10-second timeout,
Agent proxy/TLS transport, bounded JSON parsing and no redirects. Polling has jitter,
backoff and Retry-After handling; cancellation and gate rechecks prevent late grants.
AAA grants expire after two minutes, independently of RC's retained-last-success policy.
Nil discovery fails closed, rather than restoring the RC-only behavior accidentally.

Discovery demand is released at supervisor handoff, not confirmed enrollment. Registration
watching makes no AAA requests and does not recheck RC for an unchanged lifecycle. Fresh
registrations re-enable discovery; the previous lifecycle's grant is not reused. The
established Go enrollment path still owns identity handling. A deliberate core restart
with stopped children also requires fresh AAA approval, even with a saved identity.
No runtime revocation or core-owned recovery of ordinary child crashes is added.

## Validation

`dda inv test --targets=./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl,./comp/privateactionrunner/impl`

Tests retain RC-only source coverage and add real loopback RC/AAA requests for all eight
permission combinations, each with and without an existing identity file. They cover delayed
grants, dispatch/launch gate loss, expired/late grants, cancellation, backoff/Retry-After,
strict parsing and redirects. Recovery tests cover fresh/partial registration replacement,
connectivity loss, unchanged child IDs, ambiguous Start, fresh AAA after replacement, and
watch-loop cancellation. Supervisor calls are mocked; these are not Linux process or live
enrollment tests.
