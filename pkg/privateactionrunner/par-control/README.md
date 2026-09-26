# par-control

`par-control` is the Private Action Runner control plane in split mode: it runs
under `dd-procmgrd`, polls OPMS, and starts the on-demand Go executor to run actions.

## Startup

1. Rust asks `dd-procmgrd` to start `privateactionrunner run-executor`.
2. Go resolves PAR's configuration and identity (main/extra config, environment,
   secrets, Fleet policies) through the same path as the monolith.
3. Rust calls `GetControlPlaneConfig` on the executor's gRPC service for a snapshot
   (identity, OPMS URL, concurrency, headers, proxy, TLS). This doesn't wait on
   action-signing keys.

Go resolves the snapshot once per process and never re-enrolls on repeated RPCs.
When PAR or split mode is disabled, Go returns a disabled response with no identity
resolution or action init, and Rust exits cleanly. An idle executor exits on its own
timeout even before signing keys arrive.

Startup budget: 120s total, 5s per RPC, 1s retry interval. Transport errors retry;
authorization/protocol errors and executor exit fail immediately, pointing at
executor logs rather than restarting enrollment. Shutdown interrupts the wait.

Restart both processes to apply config/identity changes — there's no live config
subscription. Core Agent-only settings never override PAR-local ones, and there's
no stdout bootstrap subprocess or Rust config loader.

## Connection paths

Both processes get matching `--executor-socket` and `--ipc-cert-file` args (Go
applies them as CLI config overrides); Rust requires them and does no other
discovery.

| Deployment | Executor endpoint | Shared certificate |
|---|---|---|
| Linux packages, including Fleet | `${DD_INSTALL_DIR}/run/par-executor.sock` | `${DD_CONF_DIR}/ipc_cert.pem` |
| Linux image / Helm / Operator | `/opt/datadog-agent/run/par-executor.sock` | `${DD_IPC_CERT_FILE_PATH}` |
| Windows | `\\.\pipe\dd-par-executor` | installer-resolved `{{.EtcDir}}/ipc_cert.pem` |

The container entrypoint picks the certificate before starting procmgr: explicit
`DD_IPC_CERT_FILE_PATH`, else beside `DD_AUTH_TOKEN_FILE_PATH`, else beside the main
config — preserving existing Helm/Operator auth volumes.

```bash
par-control \
  --executor-socket /opt/datadog-agent/run/par-executor.sock \
  --ipc-cert-file /etc/datadog-agent/ipc_cert.pem
```

## Security

The config response carries the runner private key and possibly proxy credentials
and headers — never log it (Rust's `Debug` impl redacts it). Retrieval requires a
verified mTLS peer presenting the *exact* shared IPC certificate; a different
certificate signed by the same CA can submit actions but not fetch credentials.

## Build and test

Linux/Windows only. On macOS, prefix with `dda env dev run --`.

```bash
bazel test //pkg/privateactionrunner/par-control:par-control_test \
  //pkg/privateactionrunner/par-control:par-control-cli_test \
  //pkg/privateactionrunner/executor:executor_test \
  //comp/privateactionrunner/impl:impl_test
bazel build //pkg/privateactionrunner/par-control:par-control \
  //cmd/privateactionrunner:privateactionrunner
```
