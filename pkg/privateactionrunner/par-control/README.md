# par-control

`par-control` is the control-plane process for the Private Action Runner in split mode.
It runs under `dd-procmgrd` and owns the lifecycle of the on-demand executor.

`par-control` runs only when both `private_action_runner.enabled` and `private_action_runner.split_enabled` are true. These are disabled by default for now.

See this [RFC](https://docs.google.com/document/d/1VS1aI_rKRSfx9qx-bZaHJKRq8_oZdtXL9dLda93_Gmo) for details.

## Build and test

The crate is Linux/Windows-only. On macOS, use the Linux dev VM to run commands and tests:

```bash
dda env dev run -- bazel test //pkg/privateactionrunner/par-control:par-control_test
```

Bazel is the source of truth. `cargo` is supported for a faster edit/test loop,
but anything touching the generated protobuf types must be verified with Bazel,
not just cargo. Under `--cfg=bazel` the bindings come from a separate crate,
so trait impls that satisfy the orphan rule under `cargo` (where
`include_proto!` generates them locally) can fail to compile under Bazel.

On Linux:

```bash
bazel test //pkg/privateactionrunner/par-control:par-control_test
bazel build //pkg/privateactionrunner/par-control:par-control
```

Omnibus installs the binary through
`//pkg/privateactionrunner/par-control:install`. Linux and Windows E2E suites
cover package installation, process startup, split-mode ownership, and graceful
control-plane shutdown.
