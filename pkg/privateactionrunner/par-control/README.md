# par-control

`par-control` is the control-plane process for the Private Action Runner in split mode.
It runs under `dd-procmgrd` and owns the lifecycle of the on-demand executor.

`par-control` runs only when both `private_action_runner.enabled` and `private_action_runner.split_enabled` are true. These are disabled by default for now.

See this [RFC](https://docs.google.com/document/d/1VS1aI_rKRSfx9qx-bZaHJKRq8_oZdtXL9dLda93_Gmo) for details.

## Configuration

PAR owns its configuration and identity. Enabling split mode must preserve the
monolith's effective configuration, including PAR-specific files, environment,
secrets, Fleet policies, and persisted identity; Core Agent-only settings are not
an alternative source.

The short-lived Go `bootstrap-par-control` command loads configuration in PAR's
environment and supplies identity, the executor IPC certificate path, and a narrow
`runtime` snapshot: OPMS URL, concurrency, executor socket, headers, selected proxy,
and TLS settings. Rust consumes that snapshot without loading the Core Agent config
stream. The bootstrap and control binaries must come from the same Agent package.
Configuration changes require a restart. Agent version is stamped into Rust at build
time (`DD_AGENT_VERSION`); unstamped Cargo builds fall back to the crate version.

This draft restores PAR-local runtime configuration. Sharing enrollment logic with
the monolith and verifying parity across deployment modes remain follow-up work;
executor-free startup is deferred.

At startup, `par-control` runs the command passed to `--bootstrap-command` and parses
its stdout as JSON. The bootstrap command disables normal logging, while errors and
panics still use stderr. Since the payload contains credentials, stdout is never
forwarded or included in errors.

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
