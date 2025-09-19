# Rust checks

Rust-based checks.

## Structure

Each check is located in a sub-folder, with the exception of `core` that contains the shared code between every check.
The `core` folder provides a struct named `AgentCheck` to handle the communication with the Agent. 

To add new Rust-based checks, you need to create a sub-project and add its name in the main `Cargo.toml`.
Be sure to include the following files:
- `ffi.rs`: Should be included as it is.
- `lib.rs`: Only the `check()` function should be modified.

## Build a Rust-based check

Rust-based checks are meant to be compiled to shared libraries that will be loaded at runtime to execute the check's implementation.

To compile a Rust check to a shared library, you can use the following command:
```
cargo build --release -p <checkname>
```

The shared library will be then located in `target/release`.

## Side notes

This folder could be located somewhere else, like in another repository. The Agent doesn't need to have Rust checks in its repository to run these checks.
