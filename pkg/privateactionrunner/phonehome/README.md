# RC- and AAA-gated PAR startup

Core Agent owns the initial startup of packaged Linux host split-mode PAR. After RC's
last successful `Enabled && Authorized`, AAA `GET /api/v2/validate` must confirm
`private_action_runner_enroll` before core starts PAR, including a stopped runner with
a saved identity. Neither gate changes Go enrollment/persistence/retries or Rust
bootstrap. Running children and potential credential holders are left alone.

The worker is enabled by `private_action_runner.enabled` on non-Windows platforms;
there is no separate phone-home opt-in. Linux process-manager packages register both PAR children
without auto-start. Core starts the Rust control plane after its prerequisites pass;
Rust's existing bootstrap starts the Go executor. Monolithic startup, Windows and
container registrations are unchanged. Unsupported configurations report a blocked
state; they do not cause core to launch or stop a monolithic runner.

## Configuration

- Core: PAR enabled, split mode enabled, local RC enabled.
- Non-containerized, non-FIPS Linux core Agent; matching core/executor config, API key,
  site, proxy, secrets and user. RC-specific credentials/unsupported endpoint overrides
  and extra PAR config files are not supported. Live credential/endpoint changes need
  a coordinated restart.
- The standard supervisor socket is `/var/run/datadog-procmgrd/dd-procmgrd.sock`.
  `DD_PM_SOCKET_PATH` may override it with an absolute path. If overridden, configure
  the same socket for core, supervisor and children; the supervisor environment is
  not automatically inherited by children.
- Identity location uses the existing enrollment resolver: explicit
  `private_action_runner.identity_file_path`, otherwise next to `auth_token_file_path`
  when configured, otherwise next to `datadog.yaml`. The resolved path must be absolute.
- Both child registrations have `auto_start: false`. The Rust control plane retains
  `restart: on-failure` and its existing backoff; the executor retains `restart: never`.
  `restart: never` is also accepted for control-plane tests without automatic recovery.
- Optional isolated test registrations live under `testdata/processes.d`; they are not
  required for normal package startup. Core and executor must use matching configuration.
- For fresh enrollment, enable ordinary PAR API-key self-enrollment. Existing identities
  are handled by the existing PAR bootstrap, not by the gate.

## Lifecycle and limits

RC nil/false waits without starting PAR or calling AAA. A later positive RC response
allows AAA discovery without a core restart; a PAR-scope grant then permits startup. Failed RC polls retain the last successful response indefinitely;
the gate adds no RC freshness framework. The gate checks RC again immediately before Start.

After a successful handoff or observing existing active children, core watches both
supervisor registration UUIDs once a minute. A supervisor restart recreates both
registrations; only then does core re-arm startup and require RC plus a fresh AAA grant.
Replacing only one registration waits without starting anything. Losing connectivity
alone does not re-arm the gate, and a recovered connection restores the handoff state.
Unchanged registrations remain supervisor-owned even if their configuration becomes invalid.
Safe automatic recovery assumes the old supervisor's children have been terminated, as with
systemd's packaged control-group cleanup. Custom supervisors need equivalent cleanup:
new UUIDs alone do not prove that old OS processes are gone.

There is at most one accepted or ambiguous core Start request for the same observed
registration lifecycle. A definitive `NotFound` before spawn allows re-observation;
other Start errors stop the controller and require operator review, even if the
supervisor later restarts. Acknowledgement is not proof of enrollment or action readiness.
Ordinary child-process crashes/restarts keep their registration IDs: core does not
re-authorize, relaunch or stop them. The supervisor retains control-plane on-failure
recovery; Rust owns subsequent executor starts. A failed/exited child observed before
handoff requires operator review. Already-active children and potential credential
holders are left alone.

The existing enrollment implementation retains its retry/persistence behavior and limits
on ambiguous network failures. Re-authorizing after supervisor replacement re-enters the
existing bootstrap, potentially including self-enrollment if an identity was not persisted.
Neither a Start acknowledgement nor new registration IDs establish the outcome of a prior
enrollment POST. This recovery policy adds no enrollment idempotency or exactly-once guarantee.
An ambiguous Start RPC still stops automatic recovery; reconcile possibly committed enrollment
before deliberately restarting core to override that block.

Old POC `.phone-home` attempt journals block new startup rather than silently discarding
pending credentials. Preserve/reconcile old experimental state before migrating test VMs.

Internal `par_phone_home` expvar reports waiting/blocked/handoff and a bounded reason.
When PAR is enabled, `datadog-agent status` also reports the startup-gate state,
reason and next check in JSON, text and HTML.

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
