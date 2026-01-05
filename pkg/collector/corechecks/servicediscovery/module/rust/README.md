# sd-agent

A Rust implementation of resource discovery.

## Building

### Build the Binary

```bash
cargo build --release --bin sd-agent
```

The binary will be located at `target/release/sd-agent`.

### Build the Shared Library

The `dd-discovery` shared library (`libdd_discovery.so`) contains the service
discovery logic and can be linked from other languages (e.g., Go via cgo):

```bash
cargo build --release --lib
```

The shared library will be located at `target/release/libdd_discovery.so`.

**Note**: The shared library currently does not expose C-compatible FFI
functions. To use it from Go or other languages, you'll need to add FFI wrapper
functions with `#[no_mangle]` and `extern "C"` attributes.

### Build Both

```bash
cargo build --release
```

This builds both the binary and the shared library.

## Running

Start the service (requires appropriate permissions to create
`/opt/datadog-agent/run/sd-agent.sock`):

```bash
sudo ./target/release/sd-agent
```

The service listens on `/opt/datadog-agent/run/sd-agent.sock` and exposes a
single endpoint:

```
GET /discovery/services
Content-Type: application/json

{
  "heartbeat_time": 1234567890,
  "pids": [1234, 5678]
}
```

## Building with Bazel

This project also supports building with Bazel as an alternative to Cargo, in
order to ease future integration with the main datadog-agent repository.

Here are some example commands.

```bash
# Build all targets
bazel build //...

# Run all tests
bazel test //...

# Run a specific test, 10 times, with streamed output
bazel test --runs_per_test=10 //tests:dd_discovery_test --test_arg=test_nodejs_symlink --test_arg=--nocapture --test_output=streamed
```

Built binaries are located in `bazel-bin/`.

### Updating Bazel Build Files

When you make changes to `Cargo.toml`, you need to update the Bazel dependencies:

#### Adding Dependencies

1. Add the dependency to `Cargo.toml` as usual
2. Update the Bazel lockfile:
   ```bash
   CARGO_BAZEL_REPIN=1 bazel sync --only=crates
   ```
3. Add the dependency to the appropriate target in `BUILD.bazel`:
   ```starlark
   rust_binary(
       name = "sd-agent",
       deps = [
           "@crates//:your-new-crate",
           # ... other deps
       ],
   )
   ```

#### Adding New Source Files

When adding new Rust source files to the library:

1. Add the file to your project (e.g., `src/new_module.rs`)
2. Update `BUILD.bazel` to include it in the glob pattern (it should be auto-included if using `glob(["src/**/*.rs"])`)
3. For the `sd-agent` binary, explicitly add new modules if they're used by `main.rs`:
   ```starlark
   rust_binary(
       name = "sd-agent",
       srcs = [
           "src/main.rs",
           "src/new_module.rs",  # Add new files here
       ],
   )
   ```

#### Common Maintenance Tasks

- **Update Rust toolchain**: Modify `MODULE.bazel` to specify a different Rust version
- **Add test data**: Place files in `tests/testdata/` and they'll be automatically included via the `testdata` filegroup
- **Check for outdated deps**: Run `cargo update` and then re-sync Bazel with `CARGO_BAZEL_REPIN=1 bazel sync --only=crates`

### Bazel vs Cargo

Both build systems are currently maintained in parallel:
- **Cargo** is the primary build system for local development
- **Bazel** is used for reproducible builds and integration with larger Bazel-based projects
- Changes to `Cargo.toml` require updating Bazel via `CARGO_BAZEL_REPIN=1 bazel sync --only=crates`
- Test targets must be explicitly defined in Bazel (no auto-discovery like Cargo)

CI workflows ensures that both builds and tests work with both build systems.
