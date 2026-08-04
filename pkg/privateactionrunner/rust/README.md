# par-control lifecycle scaffold

`par-control` is the installed control-plane process for the split Private
Action Runner. This first slice intentionally contains only installation,
activation, and executor process lifecycle:

- `dd-procmgrd` starts `par-control` from
  `processes.d/datadog-agent-action-control.yaml`.
- Unless both `private_action_runner.enabled` and
  `private_action_runner.split_enabled` are true, `par-control` exits with code
  0 and is not restarted.
- In split mode on Linux, `par-control` asks `dd-procmgrd` to start the existing
  `privateactionrunner run-executor` process immediately. It does not connect to
  the executor or dispatch actions yet.
- On shutdown, `par-control` asks `dd-procmgrd` to stop and reap the executor.
- Windows ships the binary and process definition for package symmetry, but the
  binary exits cleanly until named-pipe transport is implemented.

OPMS polling, identity/bootstrap, action dispatch, key synchronization, idle
shutdown, and control-to-executor mTLS belong to the next slice.

## Build and test

The crate is Linux/Windows-only, so build it inside the Linux dev container on a
macOS workstation:

```bash
dda env dev run -- bash -lc 'cd /repos/datadog-agent && \
  bazel test //pkg/privateactionrunner/rust:par-control_test'
dda env dev run -- bash -lc 'cd /repos/datadog-agent && \
  bazel build //pkg/privateactionrunner/rust:par-control'
```

The binary is installed by Omnibus through
`//pkg/privateactionrunner/rust:install`; it is not part of
`dda inv privateactionrunner.build`.
