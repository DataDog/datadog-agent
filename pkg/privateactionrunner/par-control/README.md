# par-control

`par-control` is the control-plane process for the Private Action Runner in split mode.
It runs under `dd-procmgrd` and owns the lifecycle of the on-demand executor.

`par-control` runs only when both `private_action_runner.enabled` and `private_action_runner.split_enabled` are true. These are disabled by default for now.

See this [RFC](https://docs.google.com/document/d/1VS1aI_rKRSfx9qx-bZaHJKRq8_oZdtXL9dLda93_Gmo) for details.

## Configuration

`par-control` registers as a config-only Remote Agent and loads its runtime settings
from the Core Agent config stream through Saluki's `GenericConfiguration`. When split
mode is enabled, it asks the Core Agent's authenticated IPC endpoint to ensure
enrollment and return the runner identity and Agent version. An identity configured in the
local PAR extra config or environment is included in that request; a reusable identity from a
previous enrollment still takes precedence in the Core Agent.

The Core Agent IPC port, auth token path, and certificate path resolve in this order:
command-line `--cmd-port`, `--auth-token-file`, `--ipc-cert-file`; then the same three keys
read from `datadog.yaml` at its default location and the environment, layered the same way
ADP loads its own bootstrap configuration (`saluki_config::ConfigurationLoader`, environment
over file) but without ADP's generated schema or translator; then the standard Agent
locations. This resolves before `par-control` can receive configuration from the Core Agent
stream.

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
