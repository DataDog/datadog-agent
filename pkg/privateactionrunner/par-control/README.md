# par-control

`par-control` is the control-plane process for the Private Action Runner in split mode.
It runs under `dd-procmgrd` and owns the lifecycle of the on-demand executor.

`par-control` runs only when both `private_action_runner.enabled` and `private_action_runner.split_enabled` are true. These are disabled by default for now.

See this [RFC](https://docs.google.com/document/d/1VS1aI_rKRSfx9qx-bZaHJKRq8_oZdtXL9dLda93_Gmo) for details.

## Configuration

Go owns configuration and enrollment. At startup, `par-control` runs the command
passed to `--bootstrap-command` and parses its stdout as JSON. The bootstrap
command disables normal logging, while errors and panics still use stderr. Since
the payload may contain credentials, stdout is never forwarded or included in
errors.

## Build and test

The crate is Linux/Windows-only. On macOS, use the Linux dev VM to run commands and tests:

```bash
dda env dev run -- bazel test //pkg/privateactionrunner/par-control:par-control_test
```

On Linux:

```bash
bazel test //pkg/privateactionrunner/par-control:par-control_test
bazel build //pkg/privateactionrunner/par-control:par-control
```
