# par-control

`par-control` is the control-plane process for the split Private Action Runner.
It runs under `dd-procmgrd` and supervises the on-demand executor.

- `par-control` runs only when both `private_action_runner.enabled` and
  `private_action_runner.split_enabled` are true. Otherwise it exits cleanly.
- In split mode, `par-control` asks `dd-procmgrd` to start the existing
  `privateactionrunner run-executor` process and polls its state.
- Graceful shutdown uses `SIGTERM` on Linux and `CTRL_BREAK` on Windows,
  matching the events sent by `dd-procmgrd`.

## Relationship to dd-procmgrd

`par-control` and the executor are **siblings** under `dd-procmgrd`, not parent
and child. `par-control` asks the daemon to start and stop the executor over
gRPC; the daemon owns both process trees (its own process group on Unix, its own
job object on Windows). Consequences worth knowing:

- `par-control` can be restarted underneath a running executor, so `Start`
  returning `FAILED_PRECONDITION` ("already running") is a success case.
- `par-control` must **never** call back into `dd-procmgrd` from its shutdown
  path. The daemon serializes all RPCs through a single loop, which by then is
  either gone or blocked waiting for `par-control` to exit; an RPC would
  deadlock until `stop_timeout` expires and the job object kills the process.
- The socket is resolved from `DD_PM_SOCKET_PATH` before falling back to the
  platform default, matching the daemon's own `ipc_path()`.

A clean executor exit is not a failure: the executor is on-demand and exits once
idle. Only a genuine failure makes `par-control` exit non-zero and let
`dd-procmgrd` restart it.

## Build and test

The crate is Linux/Windows-only. On a macOS workstation, run the Linux dev VM:

```bash
dda env dev run -- bazel test //pkg/privateactionrunner/par-control:par-control_test
```

Bazel is the source of truth. `cargo` is supported for a faster edit/test loop,
but note two things:

- The dev VM exports an empty `PKG_CONFIG_LIBDIR`, which blanks pkg-config's
  search path, so `openssl-sys` cannot find the system OpenSSL that is in fact
  installed. Unset it for cargo:

  ```bash
  dda env dev run -- bash -c 'unset PKG_CONFIG_LIBDIR && cargo test -p par-control'
  ```

  Bazel is unaffected: it builds against the Agent's own OpenSSL through the
  `openssl-sys` annotation in `deps/crates.MODULE.bazel`.

- Anything touching the generated protobuf types must be verified with Bazel,
  not just cargo. Under `--cfg=bazel` the bindings come from a separate crate,
  so trait impls that satisfy the orphan rule under `cargo` (where
  `include_proto!` generates them locally) can fail to compile under Bazel.

- The TLS tests in `tls.rs` and `opms.rs` only pass on Linux. Running them with
  `cargo` on a macOS host fails in `Security.framework` ("Unknown format in
  import"), which rejects the PEM/SEC1 key material the tests generate. Run
  them in the Linux dev VM; Bazel never builds this crate for macOS.

On Linux:

```bash
bazel test //pkg/privateactionrunner/par-control:par-control_test
bazel build //pkg/privateactionrunner/par-control:par-control
```
