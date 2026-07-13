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
|---|---|
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

The crate builds and its unit tests pass under Bazel once `Cargo.lock` is pinned:

```
bazel test  //pkg/privateactionrunner/rust:par-control_test
bazel build //pkg/privateactionrunner/rust:par-control
```

Any change to a `Cargo.toml` (adding a crate/feature) requires regenerating the
lock file — Bazel enforces `validate_lockfile = true`:

```
cargo generate-lockfile   # from repo root, needs registry access
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
client cert. Adding `native-tls`/`tokio-native-tls` requires a `cargo
generate-lockfile` repin.

## Known follow-ups

- Validate the exact OPMS request envelopes (esp. dequeue JSON:API) against a
  running/fake OPMS; the bodies here are modeled on the Go client.
- Confirm `native_tls::Identity::from_pkcs8` accepts the IPC key (SEC1 "EC PRIVATE
  KEY") on the OpenSSL backend, and the disabled-hostname posture over the socket.
- Wire a `log` implementation (e.g. `dd-agent-log`) in `main`.
