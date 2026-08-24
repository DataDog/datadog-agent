# par-control

`par-control` is the control-plane process for the Private Action Runner in split mode.
It runs under `dd-procmgrd` and owns the lifecycle of the on-demand executor.

`par-control` runs only when both `private_action_runner.enabled` and `private_action_runner.split_enabled` are true. These are disabled by default for now.

See this [RFC](https://docs.google.com/document/d/1VS1aI_rKRSfx9qx-bZaHJKRq8_oZdtXL9dLda93_Gmo) for details.

## Configuration

Go is the configuration authority. `par-control` takes no config file of its own:
it runs the command given by `--bootstrap-command`, which is
`privateactionrunner bootstrap-par-control --cfgpath <datadog.yaml>`. That command
loads the canonical Agent configuration — local YAML, environment, Fleet policy,
secrets, endpoint and path resolution — ensures the runner is enrolled, and
returns the resolved control-plane configuration as a single
`PAR_CONTROL_CONFIG=` prefixed JSON line on stdout.

`par-control` parses that line, validates it at the trust boundary, and exits
successfully when `split_mode` is false. It does not load YAML, read Fleet policy,
decode environment variables, or resolve Agent file paths; adding any of that back
would reintroduce the Go/Rust divergence this split was built to remove. The JSON
field names and their `_milliseconds` duration units are a contract with
`cmd/privateactionrunner/subcommands/bootstrapparcontrol`.

The configuration line carries the runner private key and may carry proxy
credentials, so it is never logged and never included in an error.

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
