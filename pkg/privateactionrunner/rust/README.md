# par-control foundation

`par-control` is the standalone control-plane process for the split Private
Action Runner. This initial slice adds the Linux Rust binary, logging, and basic
Agent configuration loading.

The binary is not yet installed or supervised by `dd-procmgrd`, does not start
the action executor, and does not introduce the `split_enabled` configuration.
Those integration pieces, Windows support, and end-to-end coverage are kept in
a follow-up change.

## Build and test

On a macOS workstation, build and test the Linux-only crate in the development
container:

```bash
dda env dev run -- bazel test //pkg/privateactionrunner/rust:par-control_test
dda env dev run -- bazel build //pkg/privateactionrunner/rust:par-control
```
