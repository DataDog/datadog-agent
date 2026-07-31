# par-control

`par-control` is the always-on, minimal **control plane** of the split Private
Action Runner (PAR). It polls the on-prem management service (OPMS) for tasks,
drives the on-demand Go executor's lifecycle via the Rust process manager
(`dd-procmgrd`), dispatches actions over the local control↔executor gRPC service,
and publishes results back to OPMS. Only the control plane touches OPMS.

See `.scratch/par-rss-split/prd.md` for the full design and
`.scratch/par-rss-split/issues/01-tracer-happy-path-dispatch.md` for this slice.

## Layout

| Module | Responsibility |
| --- | --- |
| `identity.rs` | Parse the persisted runner URN + ECDSA P-256 key (Go owns enrollment). |
| `config.rs` | Load the control-plane config subset from `datadog.yaml`. |
| `jwt.rs` | `JwtSigner` trait + ES256 signer for the `X-Datadog-OnPrem-JWT` header. |
| `opms.rs` | `Opms` trait (dequeue/publish/heartbeat) + `HttpOpms` real client. |
| `procmgr.rs` | `ExecutorLifecycle` trait + `dd-procmgrd` gRPC client (Start/Describe/Stop). |
| `executor.rs` | `Dispatcher` trait + executor gRPC client (Health + RunAction stream). |
| `orchestrator.rs` | The dequeue → start → ready-gate → dispatch → publish loop, pool-paced. |
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
| `enabled: true`, `split_enabled: true` | runs (control plane) | exits 0 |

Both halves dequeue from OPMS, so exactly one may run: `config.rs::LaunchGate`
and `isSplitEnabled` in `comp/privateactionrunner/impl/privateactionrunner.go`
read the same two keys, and the loser exits **0** — a non-zero exit would trip
procmgr's/systemd's restart limit on every host that never opted in.

Identity bootstrap: in split mode the Go monolith that normally self-enrolls is
standing down, so the procmgr definition passes
`--enroll-command <privateactionrunner> rotate-identity --cfgpath <datadog.yaml>`
and par-control runs that one-shot when no identity is persisted yet (honoring
`private_action_runner.self_enroll`).

**Windows** ships the process definition for symmetry, but par-control refuses
to run in split mode there and exits 0: `transport.rs` has no named-pipe client
yet, so it could reach OPMS and dequeue tasks it can never dispatch.

## Build prerequisites (IMPORTANT)

The crate is Linux/Windows-only (`target_compatible_with`), so on a macOS
workstation build it inside the Linux dev container rather than on the host:

```
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

```
cargo metadata --format-version 1 >/dev/null   # from repo root, needs registry access
```

Do **not** use `cargo generate-lockfile` for this: it re-resolves the entire
workspace to latest-compatible and silently bumps ~80 unrelated crates shared
with //pkg/procmgr/rust, //pkg/discovery/module/rust and others.

After touching dependencies, also regenerate the license inventory (CI checks it):

```
dda inv install-rust-license-tool   # once
dda inv generate-rust-licenses
```

**TLS:** OPMS is reached with `hyper` + `native-tls` (`opms.rs`), *not* a
higher-level HTTP client. `rustls`/`ring` are intentionally avoided for
`cargo-deny`, and `ureq` — the workspace's HTTP client — cannot express what is
needed: its `native-tls` support is gated on a feature that force-enables
`native-tls-webpki-roots` (`webpki-root-certs`, CDLA-Permissive-2.0, rejected by
`cargo-deny`), and with only `native-tls-no-default` every native-tls code path in
ureq is compiled out, so *any* HTTPS request panics at runtime with "provider is
... but feature is not enabled". Cargo features are additive, so the webpki roots
cannot be subtracted. `native-tls` needs no bundled roots anyway: it locates the
system trust store at runtime via `openssl-probe`.

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
every host, so `config.rs` reproduces Go's fallback chain (`getCertFilepath` in
`pkg/api/security/cert/cert_getter.go`): explicit setting, else next to
`auth_token_file_path`, else next to `datadog.yaml`. The connector is rebuilt per
connection rather than at startup, because par-control is `auto_start: true` and
can come up before the cert exists — possibly before the very executor it launches,
which creates the cert when missing.

## Contracts with the Go side (keep in sync)

These values must match their Go counterparts exactly; a mismatch fails only at
runtime, so each is pinned by a unit test in `config.rs`.

| par-control | Must match |
| --- | --- |
| `DEFAULT_EXECUTOR_PROCESS_NAME` | the procmgr process-definition name in `pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-action-executor.yaml.tmpl` |
| `private_action_runner.executor.socket_path` | `pkg/config/setup/privateactionrunner_settings.go` (nested key, *not* flat) |
| `private_action_runner.task_concurrency` | same key and default (5) as the Go runner |
| `RUNNER_VERSION` | the agent version Go reports as `pkg/version.AgentVersion`, injected as `DD_AGENT_VERSION` by the crate-local `version.bzl` |

## Known follow-ups

- Validate the exact OPMS request envelopes (esp. dequeue JSON:API) against a
  running/fake OPMS; the bodies here are modeled on the Go client.
- `private_action_runner.opms_extra_headers` is honored by the Go client but
  ignored here.
- par-control parses `datadog.yaml` itself, so it honors none of the `DD_*`
  environment variables bound by `pkg/config/setup/privateactionrunner_settings.go`
  (including `DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED`) — a container-style
  env-only configuration will not reach the control plane.
- Config changes are only picked up on restart: neither the split switch nor the
  control-plane knobs are watched at runtime.
- Review the disabled-hostname posture on the mTLS channel over the local socket
  (`native_tls::Identity::from_pkcs8` rejecting the SEC1 IPC key is handled by
  `tls.rs::to_pkcs8_pem`).
- Wire a `log` implementation (e.g. `dd-agent-log`) in `main`. Until then the
  `log::` macros are no-ops and only the launch-path decisions in `main` are
  visible, via `eprintln!` on the stderr procmgr inherits.
