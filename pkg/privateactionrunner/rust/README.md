# par-control lifecycle scaffold

`par-control` is the installed control-plane process for the split Private
Action Runner. This slice covers installation, activation, and executor process
lifecycle only:

- `dd-procmgrd` starts `par-control` from
  `processes.d/datadog-agent-action-control.yaml`.
- `par-control` exits 0 (and is not restarted) unless both
  `private_action_runner.enabled` and `.split_enabled` are true.
- In split mode it asks `dd-procmgrd` to start the existing
  `privateactionrunner run-executor` process. It does not connect to the
  executor or dispatch actions yet.
- On shutdown it exits without calling back into its own supervisor:
  `dd-procmgrd` owns the executor and stops both processes. The shutdown trigger
  is `SIGTERM` on Linux and `CTRL_BREAK` on Windows, matching how `dd-procmgrd`
  stops children on each platform.
- The dd-procmgrd client transport is a Unix domain socket on Linux and a named
  pipe (`\\.\pipe\datadog-procmgrd`) on Windows.

The control-to-executor channel is not part of this slice; on Windows it will
need its own named pipe (`\\.\pipe\dd-par-executor`) with mTLS.

OPMS polling, identity/bootstrap, action dispatch, key synchronization, idle
shutdown, and control-to-executor mTLS belong to the next slice.

## Build and test

The crate is Linux/Windows-only, so build it inside the Linux dev container on a
macOS workstation:

```bash
dda env dev run -- bazel test //pkg/privateactionrunner/rust:par-control_test
dda env dev run -- bazel build //pkg/privateactionrunner/rust:par-control
```

The unit tests also run on Windows, where they cover the named-pipe client path.

Omnibus installs the binary through `//pkg/privateactionrunner/rust:install`; it
is not part of `dda inv privateactionrunner.build`.
