# Rust-based shared library checks

## About

This folder contains the source code for Rust-based shared library checks.

Checks can send metrics, service checks, and events to the Agent.

The entrypoint is the `Run` symbol, which receives the check configuration and callbacks to send data to the Agent with FFI using the C-ABI.

## Structure

There are 2 folders at the root level:
- `core`: Shared code between every Rust-based check, imported and used in every check. It handles FFI, callbacks and conversions between C types and Rust types.
- `checks`: Folder containing every Rust-based check, each in its own crate.

## Writing new Rust-based checks

To start writing a new Rust check, follow these steps:
- In `checks`, copy the crate `example` and rename it with your check name. The check 'example' is a template for Rust-based checks.
- Change the crate name in `Cargo.toml`.
- Add your crate in `workspace.members` in the `Cargo.toml` located at the root level.
- Write your implementation in the `check` function in `check.rs`. 

And you're done!

Alternatively, you can create the crate with `cargo new checks/<check_name>` then copy the `Cargo.toml` and the source files manually. `cargo new` will automatically add your crate in the workspace members.

## Compiling into shared libraries

To run Rust checks in the Agent, you need to compile them into a C-Shared library with:

```
cargo build --release --package <check_name>
```

The shared library will be created in `target/release` under the name `lib<check_name>.<lib_extension>`.

## Testing Rust-based shared library checks

Directly with the Agent by scheduling the check.

It will be done later with a standalone binary that prints payloads sent by checks.

## Additional notes

- The project is experimental. The logic and behavior of Rust-based checks may change.
- For more information about shared library checks, see [#39676](https://github.com/DataDog/datadog-agent/pull/39676).
