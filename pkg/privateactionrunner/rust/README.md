# par-control lifecycle scaffold

`par-control` is the installed control-plane process for the split Private
Action Runner. This slice covers packaging, activation, and executor process
supervision on Linux and Windows:

- Non-FIPS Agent packages install `par-control` and auto-start it through the
  `processes.d/datadog-agent-par-control.yaml` definition. The executor has a
  separate, non-auto-start process definition.
- `par-control` runs only when both `private_action_runner.enabled` and
  `private_action_runner.split_enabled` are true. Otherwise it exits 0 and
  `restart: on-failure` leaves it stopped. It resolves those settings and the
  log level with Fleet policy overriding environment variables, which override
  local YAML.
- On Linux and Windows, the monolithic Go runner also exits cleanly when split
  mode is enabled. Other platforms continue to use the monolithic runner.
- In split mode, `par-control` asks `dd-procmgrd` to start the existing
  `privateactionrunner run-executor` process and polls its state. An already
  running executor is accepted. A terminal executor state or process-manager
  error makes `par-control` fail so `dd-procmgrd` can restart the control plane,
  which starts the executor again.
- Stopping `par-control` does **not** stop the executor. The control plane exits
  without making a nested RPC into the supervisor, and the independently
  managed executor remains running. `dd-procmgrd` stops each process when the
  daemon itself shuts down.
- Graceful shutdown uses `SIGTERM` on Linux and `CTRL_BREAK` on Windows,
  matching the events sent by `dd-procmgrd`. The process-manager client uses a
  Unix domain socket on Linux and `\\.\pipe\datadog-procmgrd` on Windows.
- Linux diagnostics go to the inherited process-manager streams; Windows logs
  go to `logs/par-control.log` under the directory containing `datadog.yaml`.

This branch does not add a control-to-executor connection or action dispatch.
OPMS polling, identity/bootstrap, key synchronization, idle shutdown, and the
mTLS control-to-executor channel remain out of scope.

## Build and test

The crate is Linux/Windows-only, so build it inside the Linux dev container on a
macOS workstation:

```bash
dda env dev run -- bazel test //pkg/privateactionrunner/rust:par-control_test
dda env dev run -- bazel build //pkg/privateactionrunner/rust:par-control
```

The unit tests also run on Windows, where they cover the named-pipe client path.
Linux and Windows E2E suites cover installation, process startup, and graceful
control-plane shutdown while confirming that the executor remains running.
The Windows suite also verifies that the monolithic runner stands down.

Omnibus installs the binary through `//pkg/privateactionrunner/rust:install`; it
is not part of `dda inv privateactionrunner.build`. As a supporting
process-manager change, Unix children now receive a standard service `PATH`
after `dd-procmgrd` clears its inherited environment.
