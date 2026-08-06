# par-control

`par-control` is a new control-plane for the Private Action Runner.

## Build and test

```bash
dda env dev run -- bazel test //pkg/privateactionrunner/rust:par-control_test
dda env dev run -- bazel build //pkg/privateactionrunner/rust:par-control
```
