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

There is **at most one Start RPC per core process lifetime**. Acknowledgement means only
that startup was handed to the supervisor, not that enrollment or action readiness succeeded.
After a successful or ambiguous Start, this controller does not retry or monitor enrollment.
Already-active children are left untouched. After handoff, the supervisor owns
control-plane crash recovery, without rechecking RC/AAA. Rust owns subsequent executor
starts. Core never stops an action-serving PAR or a process that might hold credentials.
A failed/exited child observed before handoff requires operator review.

The existing enrollment implementation retains its own retry/persistence behavior and its
existing limitations on ambiguous network failures. The gate adds no new enrollment POST
retry, journal, idempotency promise or credential-recovery policy. An operator restart can
re-arm the gate: reconcile any possibly committed enrollment before manually retrying. The
control-plane on-failure policy can re-enter the existing bootstrap/enrollment path;
this is unchanged crash-recovery behavior, not an enrollment idempotency guarantee.
Restart core after a supervisor daemon restart: the one-shot gate
does not monitor or re-arm itself for recreated supervisor registrations.

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

Discovery ends when core hands startup to the supervisor, not when enrollment is
confirmed. The established Go enrollment path owns everything after that handoff.
A subsequent deliberate core restart with stopped children requires fresh AAA approval,
even with a saved identity. No post-enrollment revocation or core-owned automatic relaunch is added.

## Validation

`dda inv test --targets=./pkg/privateactionrunner/phonehome,./comp/privateactionrunner/phonehome/impl,./comp/privateactionrunner/impl`

Tests retain RC-only source coverage and add real loopback RC/AAA requests for all eight
permission combinations, each with and without an existing identity file. They cover delayed
grants, dispatch/launch gate loss, expired/late grants, cancellation, backoff/Retry-After,
strict parsing and redirects. Supervisor calls are mocked; these are not Linux process or
live enrollment tests.
