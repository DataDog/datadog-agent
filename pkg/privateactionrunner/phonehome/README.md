# Core-owned PAR phone-home POC

Opt-in experiment, not a rollout/default-on change. Based on Agent
`1cd4a3d618bc3b02219d8a8dbfc5613160fe7fe4`. No RC/backend changes or Rust credential
handoff. This directory implements eligibility, **not action readiness**.

## Contracts

- **Scope:** one non-containerized Linux node Agent, non-FIPS, split mode, local
  `dd-procmgrd`. Other platforms, flavors, Kubernetes and arbitrary PAR-local
  config are not supported by this POC.
- **Opt-in:** `DD_PAR_PHONE_HOME_POC=true` in core and executor. PAR must also be
  locally enabled and `private_action_runner.split_enabled: true`. Without the
  environment opt-in, existing behavior is unchanged. Local disablement makes
  the controller a no-op; this is not a new policy for stopping a running PAR.
- **Matching config:** same main file, environment, secret resolutions, hostname,
  user, filesystem and identity path for core and executor. No extra config
  files, PAR-specific credentials, Fleet overrides, per-process proxies, or live
  config/secret rotation. Restart both processes after configuration changes.
  Core uses its already-resolved config; it does **not** load or overwrite PAR's
  config. The environment opt-in is the operator's attestation of this setup,
  not proof of arbitrary config parity. The known PAR extra-config environment
  variable and OPMS extra headers are explicitly rejected by core.
- **Launch:** set `DD_PM_SOCKET_PATH` to an explicit absolute path for the isolated
  supervisor. Register `datadog-agent-par-control` and
  `datadog-agent-action-executor` with `auto_start: false`, `restart: never`.
  Executor's process definition must explicitly contain
  `env: {DD_PAR_PHONE_HOME_POC: "true"}` so core can check that the POST guard is
  enabled. Core inspects these policies and uses Describe/Start on registered
  processes only; no Create, shell commands, or runtime policy rewriting.
  Do not run another systemd/s6/kubelet/monolithic PAR launch path in parallel.
- **Discovery:** API-key-only GET `/api/v2/validate`. Both `valid: true` and
  `private_action_runner_enroll` are required; RC scope is irrelevant. Agent HTTP
  transport preserves configured TLS/proxy behavior. Redirects are refused;
  HTTP bodies, transport error strings and API keys are not copied to status.
- **Identity:** use the same enrollment identity reader and `ShouldReenroll`
  logic as Go PAR, including the current hostname/API-key hash checks. Complete
  legacy app-key identities without hashes are reusable. Existing identities
  bypass discovery even when the existing Go path needs to re-enroll. Inline
  identities are supported but are not newly persisted by the POC.
- **Outcome:** core checks the existing identity file (including private JWK/URN
  validation), not a PID or repeated bootstrap RPC. The executor still generates
  keys, enrolls and persists. Enrollment rejection is an error in POC split mode,
  not a disabled snapshot. Identity resolution has a 45-second budget; a single
  POST has a 30-second budget, within Rust's unchanged 120-second bootstrap limit.
  After persistence and control startup, discovery stops. Signing-key/RC readiness
  is separate and is not claimed.

## Retry and recovery policy

Missing scope: 60–72 seconds between checks, with 0–60 seconds initial jitter.
Transport/5xx/malformed responses: exponential base delay 1, 2, 4, 8, 15 minutes,
plus up to 12 seconds jitter. Invalid credentials and other HTTP failures: 15
minutes plus jitter. Discovery 429 honors delta-seconds or HTTP-date Retry-After
as a lower bound. A discovery 403 is **not** interpreted as missing PAR scope.

Core creates `<identity_file_path>.phone-home/` with mode 0700 before Start. Go
exclusively creates and syncs its empty `post` marker before sending enrollment.
This stores **no API key, private key, token, or credential hash**. A fixed
`outcome` reason records failure; unknown/missing outcomes remain ambiguous.
The directory survives core/executor crashes and blocks duplicate attempts. Once
core confirms a reusable identity and running control, it removes the known
journal files and directory so a later hostname/key change can use the existing
re-enrollment path (after restarting both processes).

**Deliberate POC limitation:** there is at most one enrollment POST per authorized
attempt, including for definitive 401/403/429 rejection. There is no automatic
POST retry or process-restart loop. Discovery continues (normally once per minute
while launching, slowly after failure); it does not authorize a second attempt.
A known reusable identity can be started once per core lifetime and does not need
another scope check. A running PAR is never stopped on discovery failure.

For manual recovery, first stop the isolated core/controller and both PAR
processes. Inspect the outcome and backend registration. For a lost response,
5xx or local persistence failure, do **not** simply remove the journal: first
reconcile any created backend runner and recover the identity or explicitly
clean up that registration. Only after confirming that a new attempt is safe,
remove this POC's attempt directory and restart core. Never delete the existing
identity as a recovery shortcut. This policy trades availability for avoiding
orphan registrations until a real idempotency/recovery contract is designed.

The journal protects process restarts, not a distributed filesystem or guaranteed
power-loss durability. Use a private local directory writable only by the test
Agent user. No cross-host shared identities are supported.

## Reproducible fake-backend scenario (Linux or macOS test host)

No credentials, cloud VM, host services or real API endpoints are needed:

```sh
dda inv test --targets=./pkg/privateactionrunner/phonehome \
  --test-args='-test.run=TestPhoneHomeScenario -test.v' \
  --bazel-args='--jobs=4 --test_output=all'
```

The test uses a loopback HTTP server, the real Go enrollment client/key generation
and identity persistence, and a supervisor fake. It advances the controller's
clock rather than sleeping for minutes:

1. Three unscoped checks: zero Start calls and zero enrollment POSTs.
2. Grant scope: exactly one Start of registered Rust control.
3. Keep enrollment pending: discovery still happens; a running PID is not success.
4. Execute existing Go enrollment and persist identity: state becomes enrolled,
   polling stops without core restart.
5. Restart the controller with scope absent: reuse identity and running process,
   with no extra Start or POST.

Additional tests exercise legacy identities/re-enrollment, concurrency, shutdown
and in-flight cancellation, invalid keys, 403, outages/backoff, 429 Retry-After,
redirect protection, and ambiguous/rejected outcomes across controller restarts.
The component tests call the actual Go `getRunnerConfig`/`configureExecutor`
paths for successful enrollment, rejection, 5xx, lost response and persistence failure.

```sh
dda inv test --targets=./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment,./pkg/privateactionrunner/opms,./comp/privateactionrunner/impl,./comp/privateactionrunner/status/statusimpl \
  --bazel-args='--jobs=4 --test_output=errors'
dda inv test --targets=./pkg/privateactionrunner/phonehome,./pkg/privateactionrunner/enrollment \
  --race --bazel-args='--jobs=4 --test_output=errors'
dda inv test --targets=./cmd/agent/subcommands/run \
  --bazel-args='--jobs=4 --test_output=errors'
```

On a Linux development host these use the same tests and fakes. On macOS a Linux
dev environment can run them with `dda env dev run -- dda inv test ...`, after
setting up that environment. Do not substitute raw `go test`.

## Isolated real-process wiring (not executed)

`testdata/processes.d/` contains POC-only replacements for the two PAR definitions;
shipping package/image templates are intentionally unchanged. In a separately
approved non-containerized Linux sandbox, use these in a dedicated
`DD_PM_CONFIG_DIR`, an isolated `DD_PM_SOCKET_PATH`, `DD_PAR_POC_BIN` pointing at
binaries built from this branch, and `DD_PAR_POC_DIR` pointing at a private temp
directory. Start core separately with that same environment and config, not via
host service installation. Core and executor must share the IPC certificate and
hostname. Do not point this experiment at a customer/production key.

Example config shape (substitute private sandbox absolute paths):

```yaml
api_key: fake-test-key
site: datadoghq.com
# Local mock only; with DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS=true in both Go processes.
dd_url: http://127.0.0.1:18080
remote_configuration:
  enabled: false
private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: true
  api_key_only_enrollment: true
  identity_file_path: /absolute/sandbox/identity.json
```

Discovery permits plain HTTP only for loopback mocks. A real-process mock would
also need to serve Rust OPMS traffic after bootstrap; the Go-only fake scenario
above does not model that traffic. These files are wiring examples, **not an
end-to-end real-process harness**.

## Observability and validation boundary

Core `agent status` (text, HTML and JSON) and expvar `par_phone_home` expose
waiting/launching/enrolled/blocked, fixed reason, and next reconciliation time.
No enrollment failure affects core startup or health. `enrolled` does not mean
that actions can execute.

Validated on macOS arm64: the combined focused/core run-command suites passed
8 Bazel test targets; the final controller/enrollment race run passed 3 targets.
A final enrollment-message change was followed by passing OPMS/component tests
again. Gazelle/buildifier and `git diff --check` passed. No complete core daemon, Rust control, executor process tree, or real
supervisor was launched; no live enrollment was performed. Linux daemon/socket
permissions, waiting RSS, real process count, wall-clock request cadence and
scope-grant-to-enrollment latency remain **unmeasured**. Fake counters prove calls
and state transitions, not production memory/liveness behavior.

## Production follow-ups

- Packaging: conditional launch ownership in package/image installers, upgrades,
  core-disabled fallback, and post-enrollment crash supervision. Kubernetes
  containers are started by kubelet; this does not remove their footprint.
- Config: effective PAR-local extra files/env/secrets/Fleet config parity, safe
  secret/key rotation, site/proxy refresh, and authenticated shared config.
- Recovery: safe bounded retries of definitive rejection; idempotent enrollment,
  stable request identity and recovery after backend success/response loss/local
  persistence failure; atomic identity publication and power-loss durability.
- Supervisor: Linux socket ownership/permissions, least-privilege mutation API,
  binary-version compatibility, upgrades, readiness and lifecycle races.
- Revocation: verify backend behavior after scope/key removal; no new
  post-enrollment enforcement guarantee is introduced here.
- Capacity/rollout: AAA approval, per-site limits and fleet sizing before any
  broader opt-in; split-default prerequisite, other OS/flavor/FIPS support.
