# par-control

`par-control` is the always-on control-plane process for the split Private
Action Runner. It owns communication with the on-prem management service (OPMS)
and starts the existing Go executor on demand through `dd-procmgrd`.

The split mode is opt-in. `par-control` runs only when both
`private_action_runner.enabled` and `private_action_runner.split_enabled` are
true; otherwise it exits cleanly so the monolithic runner remains the sole OPMS
consumer.

In split mode, the control plane:

- loads the Agent's effective configuration and bootstraps the persisted runner
  identity through the existing Go enrollment flow;
- authenticates OPMS requests with the runner's ES256 identity;
- polls OPMS for tasks and reports health, heartbeats, and terminal outcomes;
- starts and stops `privateactionrunner run-executor` through `dd-procmgrd`;
- synchronizes task-signing keys and dispatches actions over a local mTLS gRPC
  channel secured by the Agent IPC certificate;
- bounds concurrency, retries transient failures, shuts down an idle executor,
  and drains in-flight work during control-plane shutdown.

Packaging and activation of the binary are handled separately from this crate.

## Build and test

The crate supports Linux and Windows, while its Bazel test target currently runs
on Linux because the Windows test executable does not stage the Agent OpenSSL
DLLs. On a macOS workstation, build and test it inside the Linux dev container:

```bash
bazel test //pkg/privateactionrunner/par-control:par-control_test
bazel build //pkg/privateactionrunner/par-control:par-control
```
