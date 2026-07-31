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

**TLS:** the crate enables `ureq`'s `native-tls` feature for real HTTPS to OPMS —
`rustls`/`ring` are intentionally avoided for `cargo-deny` (OpenSSL is Apache-2.0,
allowed). The resulting `openssl-sys` links the agent's own OpenSSL
(`@openssl//:openssl`, built from source via foreign_cc — "same as the rest of the
agent"), wired by a `crate.annotation` in `deps/crates.MODULE.bazel` that points
the build script at the foreign_cc install tree (`@openssl//:gen_dir`) via
`OPENSSL_DIR`. No system OpenSSL or Rust-vendored copy is used.

**mTLS (slice 7):** the control<->executor channel is secured with mutual TLS via
the agent IPC cert. par-control reads the combined IPC cert/key file
(`ipc_cert_file_path`) and presents it as its client identity over the socket
(`tls.rs` + `transport::connect_lazy_tls`), using native-tls (OpenSSL) for the
same cargo-deny reasons as the OPMS client. The executor requires a CA-signed
client cert. Adding `native-tls`/`tokio-native-tls` requires a lock-file repin
(see above).

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
- The par-control-only knobs (`procmgr_socket_path`, `executor_process_name`,
  `idle_timeout_seconds`, `heartbeat_interval_seconds`) are not registered in
  `pkg/config/schema/yaml/private_action_runner.yaml`, so they are undocumented
  and unvalidated.
- Confirm `native_tls::Identity::from_pkcs8` accepts the IPC key (SEC1 "EC PRIVATE
  KEY") on the OpenSSL backend, and the disabled-hostname posture over the socket.
- Wire a `log` implementation (e.g. `dd-agent-log`) in `main`.
