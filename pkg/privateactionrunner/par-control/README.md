# par-control

`par-control` is the Private Action Runner control plane in split mode. It runs
under `dd-procmgrd`, polls OPMS and starts the on-demand Go executor for actions.

## Startup and configuration

1. Rust asks `dd-procmgrd` to start `privateactionrunner run-executor`.
2. Go loads PAR's main/extra configuration, environment, secrets and Fleet policies.
   It selects identity through the same resolution/enrollment path as the monolith.
3. Rust fetches `GetControlPlaneConfig` over the executor's authenticated gRPC service.
   The snapshot contains identity, OPMS URL, concurrency, headers, selected proxy and
   TLS settings. It does not depend on action-signing keys being ready.

Go resolves the snapshot once per process; repeated RPCs do not enroll again. When
PAR or split mode is disabled, Go returns a disabled response without resolving
identity or initializing actions. Rust exits cleanly. An idle executor exits even
when no signing keys have arrived; disabled bootstrap always has an idle timeout.

Startup has a 120-second budget, 5-second configuration request timeout and 1-second
retry interval. Transport failures are retried; authorization/protocol errors fail
immediately. If the executor exits before returning configuration, control fails
with a pointer to executor logs rather than repeatedly restarting enrollment.
Shutdown interrupts startup waits.

PAR owns its configuration and identity; Core Agent-only settings are not a fallback.
Restart both control and executor to apply configuration changes. Agent version
remains stamped into Rust (`DD_AGENT_VERSION`), with a crate-version fallback for
unstamped Cargo builds. No stdout bootstrap subprocess or Rust config loader is used.

## Connection paths

Process definitions supply matching `--executor-socket` and `--ipc-cert-file`
arguments to both processes. Go applies these as CLI config overrides. Rust requires
both; it does not discover them through YAML, secrets or Fleet policies.

Linux packages use their active install/config directories. Windows uses the
installer-resolved data directory and `\\.\pipe\dd-par-executor`. In containers,
the entrypoint selects the certificate before starting procmgr: explicit
`DD_IPC_CERT_FILE_PATH`, otherwise beside `DD_AUTH_TOKEN_FILE_PATH`, otherwise beside
the main config. This preserves existing Helm/Operator auth volumes. Config-only
custom certificate locations are outside this launch contract.

```bash
par-control \
  --executor-socket /opt/datadog-agent/run/par-executor.sock \
  --ipc-cert-file /etc/datadog-agent/ipc_cert.pem
```

## Security

The configuration response contains the runner private key and potentially proxy
credentials and sensitive headers. Never log the response; Rust wraps the generated
protobuf with credential-safe `Debug` output.

Configuration retrieval requires a verified mTLS peer presenting the **exact shared
IPC certificate**. A different certificate signed by the same CA may submit signed
actions, but cannot retrieve credentials. This trusts holders of the shared Agent
IPC private key; it does not distinguish individual Agent processes.

## Build and test

The crate is Linux/Windows-only. On macOS, prefix these commands with
`dda env dev run --` to use the Linux dev environment:

```bash
bazel test //pkg/privateactionrunner/par-control:par-control_test \
  //pkg/privateactionrunner/par-control:par-control-cli_test \
  //pkg/privateactionrunner/executor:executor_test \
  //comp/privateactionrunner/impl:impl_test
bazel build //pkg/privateactionrunner/par-control:par-control \
  //cmd/privateactionrunner:privateactionrunner
```
