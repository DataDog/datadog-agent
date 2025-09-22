# Rust checks

For more details about the implementation of Rust checks, check this [Confluence page](https://datadoghq.atlassian.net/wiki/spaces/ARUN/pages/5479301643/Shared+libraries+checks+v1).

## Structure

Each check is a sub-folder, with the exception of `core` that isn't a check but rahter contains the shared code between every check.

## Writing a Rust check

To start writing a new Rust-based check, you have 2 options:
- Copy the folder `example`, which is an "Hello world" Rust check.
- Create a new Rust sub-project and copy `ffi.rs` and `lib.rs` from any checks you want to your sub-project source.

Then, follow these steps:
- Make sure to change the name of the package in the `Cargo.toml`.
- Leave `ffi.rs` as it is.
- Write any code you want in the `check` function in `lib.rs`, this is the function that contains the check's custom implementation.

And you're done! 

## Building a Rust check

Building a Rust check means compiling it into a C shared library.

To compile a Rust check into a shared library, you can use the following command:
```
cargo build --release -p <checkname>
```

You can then find the shared library in `target/release`.

## Side notes

This folder could be located somewhere else, like in another repository. The Agent doesn't need to have Rust checks in its repository to be able to run this type of checks.
