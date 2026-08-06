# par-control lifecycle scaffold

`par-control` is the control-plane process for the split Private Action Runner.
This slice covers configuration and executor process supervision on Linux and
Windows without adding the binary to Agent packages.

- `par-control` runs only when both `private_action_runner.enabled` and
  `private_action_runner.split_enabled` are true. Otherwise it exits cleanly.
- In split mode, `par-control` asks `dd-procmgrd` to start the existing
  `privateactionrunner run-executor` process and polls its state.
- Graceful shutdown uses `SIGTERM` on Linux and `CTRL_BREAK` on Windows,
  matching the events sent by `dd-procmgrd`.

This branch does not package or activate `par-control`, and it does not add a
control-to-executor connection or action dispatch. OPMS polling,
identity/bootstrap, key synchronization, idle shutdown, and the mTLS
control-to-executor channel remain out of scope.

## Build and test

The crate is Linux/Windows-only, so build it inside the Linux dev container on a
macOS workstation:

```bash
bazel test //pkg/privateactionrunner/par-control:par-control_test
bazel build //pkg/privateactionrunner/par-control:par-control
```
