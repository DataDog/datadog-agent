# par-control

`par-control` is the always-on, minimal **control plane** of the split Private
Action Runner (PAR). It polls the on-prem management service (OPMS) for tasks,
drives the on-demand Go executor's lifecycle via the Rust process manager
(`dd-procmgrd`), dispatches actions over the local control↔executor gRPC service,
and publishes results back to OPMS. Only the control plane touches OPMS.

## Layout

| Module | Responsibility |
| --- | --- |
| `identity.rs` | Parse the persisted runner URN + ECDSA P-256 key (Go owns enrollment). |
| `config.rs` | Load the Go Agent's effective control-plane config snapshot; read the raw YAML only for the launch gate. |
| `jwt.rs` | `JwtSigner` trait + ES256 signer for the `X-Datadog-OnPrem-JWT` header. |
| `opms.rs` | `Opms` trait (dequeue/publish/heartbeat/health-check) + `HttpOpms` real client. |
| `procmgr.rs` | `ExecutorLifecycle` trait + `dd-procmgrd` gRPC client (Start/Describe/Stop). |
| `executor.rs` | `Dispatcher` trait + executor gRPC client (Health + RunAction stream). |
| `orchestrator.rs` | The dequeue → start → ready-gate → dispatch → publish loop, pool-paced, plus the OPMS runner health-check loop. |
| `proto.rs` | Bazel/cargo dual proto wiring (procmgr + executor prost crates). |
| `transport.rs` | Lazily-connecting UDS client channel. |

## How it gets launched

`par-control` is supervised by `dd-procmgrd`, from the process definition
`processes.d/datadog-agent-action-control.yaml`
(`pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-action-control.yaml.tmpl`,
plus the `-windows` variant). It is `auto_start: true` and installed on every
host that ships the binary, so the *binary* owns the decision to run:

| `private_action_runner` config | `par-control` | Go `privateactionrunner run` |
| --- | --- | --- |
| `enabled: false` | exits 0 | exits 0 |
| `enabled: true` (default) | exits 0 | runs (monolithic) |
| `enabled: true`, `split_enabled: true` | runs (control plane) | exits 0 on Linux |

Both halves dequeue from OPMS, so exactly one may run: `config.rs::LaunchGate`
and `isSplitEnabled` in `comp/privateactionrunner/impl/privateactionrunner.go`
read the same two keys, and the loser exits **0** — a non-zero exit would trip
procmgr's/systemd's restart limit on every host that never opted in.

Effective config and identity bootstrap: in split mode the Go monolith that
normally loads config and self-enrolls is standing down. The process definition
passes the existing Go binary as `--config-helper`; par-control invokes its hidden
`resolve-control-config` subcommand once at startup and consumes the JSON snapshot
from stdout. This preserves the Agent's environment precedence, secret backend,
Fleet Policy, FIPS, proxy, and config-transform behavior without persisting a
second plaintext config file. If no identity is available, par-control runs
`--enroll-command <privateactionrunner> rotate-identity --cfgpath <datadog.yaml>`
and resolves the snapshot again (honoring `private_action_runner.self_enroll`).

**Windows** ships the process definition for package symmetry, but split mode is
not supported until `transport.rs` has a named-pipe client. `par-control` exits
without polling and the Go monolith remains active. Darwin also keeps the Go
monolith because `par-control` is not packaged there.

On shutdown, `par-control` drains its in-flight work and exits without issuing a
nested Stop RPC to `dd-procmgrd`: the supervisor may already be synchronously
waiting for `par-control` to exit. `dd-procmgrd` retains ownership of the
executor and stops both processes when the supervisor shuts down. Executor Stop
RPCs are reserved for polling-informed idle reaping while the supervisor command
loop is available.

## Build prerequisites (IMPORTANT)

The crate is Linux/Windows-only (`target_compatible_with`), so on a macOS
workstation build it inside the Linux dev container rather than on the host:

```bash
dda env dev run -- bash -lc 'cd /repos/datadog-agent && \
  bazel test //pkg/privateactionrunner/rust:par-control_test'
dda env dev run -- bash -lc 'cd /repos/datadog-agent && \
  bazel build //pkg/privateactionrunner/rust:par-control'
```

Note that par-control is not part of `dda inv privateactionrunner.build`; omnibus
installs it by calling `bazel run //pkg/privateactionrunner/rust:install`
directly, so use the Bazel targets above when iterating.

Any change to a `Cargo.toml` (adding a crate/feature) requires updating the lock
file — Bazel enforces `validate_lockfile = true`. Use a command that performs a
*minimal* resolution, which adds the missing crates while leaving every existing
pin untouched:

```bash
cargo metadata --format-version 1 >/dev/null   # from repo root, needs registry access
```

Do **not** use `cargo generate-lockfile` for this: it re-resolves the entire
workspace to latest-compatible and silently bumps ~80 unrelated crates shared
with //pkg/procmgr/rust, //pkg/discovery/module/rust and others.

After touching dependencies, also regenerate the license inventory (CI checks it):

```bash
dda inv install-rust-license-tool   # once
dda inv generate-rust-licenses
```

**TLS and proxying:** OPMS is reached with `reqwest` configured with default
features disabled and only `native-tls` enabled. This provides pooled HTTP/1.1,
HTTP CONNECT proxy support, proxy authentication, and stale-connection recovery
without pulling in rustls/ring/webpki roots. The client receives the Agent's
effective proxy and TLS settings from the Go helper, including secret-backed
proxy credentials, no-proxy behavior, `skip_ssl_validation`, and
`min_tls_version`.

`ureq` — the workspace's other HTTP client — cannot express what is needed: its
`native-tls` support is gated on a feature that force-enables
`native-tls-webpki-roots` (`webpki-root-certs`, CDLA-Permissive-2.0, rejected by
`cargo-deny`), and with only `native-tls-no-default` every native-tls code path in
ureq is compiled out. Cargo features are additive, so the webpki roots cannot be
subtracted. `native-tls` needs no bundled roots anyway: it locates the system
trust store at runtime via `openssl-probe`.

This makes par-control the first Rust component in the agent to make an *outbound*
HTTPS request — `cmd/ai_prompt_logger` deliberately POSTs plaintext to the local
trace agent and lets Go do the TLS leg, and the other crates only serve local
sockets. There is no unit test that can prove HTTPS works against a real endpoint;
`opms::tests::round_trips_over_a_real_tls_connection` covers it hermetically with a
local native-tls server, and it must stay that way.

The resulting `openssl-sys` links the agent's own OpenSSL
(`@openssl//:openssl`, built from source via foreign_cc — "same as the rest of the
agent"), wired by a `crate.annotation` in `deps/crates.MODULE.bazel` that points
the build script at the foreign_cc install tree (`@openssl//:gen_dir`) via
`OPENSSL_DIR`. No system OpenSSL or Rust-vendored copy is used.

**mTLS (slice 7):** the control<->executor channel is secured with mutual TLS via
the agent IPC cert. par-control reads the combined IPC cert/key file
(`ipc_cert_file_path`) and presents it as its client identity over the socket
(`tls.rs` + `transport::connect_lazy_tls`), using the same native-tls (OpenSSL)
stack as the OPMS client. The executor requires a CA-signed client cert.

`ipc_cert_file_path` defaults to empty in the agent and is unset on virtually
every host. The Go effective-config helper resolves the same fallback chain as
`getCertFilepath` in `pkg/api/security/cert/cert_getter.go`: explicit setting,
else next to `auth_token_file_path`, else next to `datadog.yaml`. The connector is
rebuilt per connection rather than at startup, because par-control is
`auto_start: true` and can come up before the cert exists — possibly before the
very executor it launches, which creates the cert when missing.

## Contracts with the Go side (keep in sync)

These values must match their Go counterparts exactly; a mismatch fails only at
runtime, so each is pinned by a unit test in `config.rs`.

| par-control | Must match |
| --- | --- |
| `DEFAULT_EXECUTOR_PROCESS_NAME` | the procmgr process-definition name in `pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-action-executor.yaml.tmpl` |
| `private_action_runner.executor.socket_path` | `pkg/config/setup/privateactionrunner_settings.go` (nested key, *not* flat) |
| `private_action_runner.task_concurrency` | same key and default (5) as the Go runner |
| `HEALTH_CHECK_INTERVAL` | `healthCheckInterval` (30s) in `pkg/privateactionrunner/adapters/config/constants.go` |
| `RUNNER_VERSION` | the agent version Go reports as `pkg/version.AgentVersion`, injected as `DD_AGENT_VERSION` by the crate-local `version.bzl` |

## Runner liveness

Besides task flow, par-control runs the runner health-check loop that the Go
monolith runs in `runners.CommonRunner`: a signed `GET` on
`/api/v2/on-prem-management-service/runner/health-check` every 30 seconds. In
split mode the monolith stands down, so if the control plane skipped this, an
idle split-mode host would emit no liveness signal at all.

The loop deliberately mirrors the Go one:

- it starts before executor pre-warm and is independent of key readiness, so a
  runner blocked waiting on its executor still reports in;
- it never aborts — a failing check is logged and retried on the next tick,
  because giving up would make a working runner look permanently dead;
- `X-Retry-After-Ms` paces the next check even when the check was *rejected*, so
  a throttling OPMS is not answered with more traffic (bounded by the same
  two-minute cap as dequeue);
- successes log at info at most once per 10 minutes (Go's `LogLimit(1, 10m)`) and
  at debug otherwise; `X-Server-Time` is logged like Go does.

It is *not* operator-tunable, matching Go: OPMS reasons about runner liveness
from this call, so both deployment modes must report at one rate.

Statsd parity is still open: the Go loop also emits the non-billable gauges
`datadog.actions.private_actions.runner.running` and
`datadog.actions.private_runner.health.check` (see
`pkg/privateactionrunner/observability/metrics.go`). par-control has no metrics
sink yet, and the on-demand executor cannot cover them because it is stopped
while idle — exactly when "is this runner running" matters.

## Known follow-ups

- Validate the exact OPMS request envelopes (esp. dequeue JSON:API) against a
  running/fake OPMS; the bodies here are modeled on the Go client.
- The launch gate and initial log level are intentionally read directly from
  `datadog.yaml`/environment before invoking the Go helper; neither requires
  secret-backed values.
- Config changes are only picked up on restart: neither the split switch nor the
  control-plane knobs are watched at runtime.
- Review the disabled-hostname posture on the mTLS channel over the local socket
  (`native_tls::Identity::from_pkcs8` rejecting the SEC1 IPC key is handled by
  `tls.rs::to_pkcs8_pem`).
