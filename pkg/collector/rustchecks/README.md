# Rust checks

For more details about the implementation of Rust checks, check this [Confluence page](https://datadoghq.atlassian.net/wiki/spaces/ARUN/pages/5479301643/Shared+libraries+checks+v1).

## Structure

There are 2 folders at the root level:
- `core`: Shared code between every Rust check. This code is statically linked to every check build.
- `checks`: Location of every Rust check, each has its own folder.

## Writing a Rust check

To start writing a new Rust check, you have 2 options:
- Make a copy of `example` in `checks`, which is an "Hello world" Rust check.
- Create a new Rust subproject in `checks` and copy `ffi.rs` and `lib.rs` from any checks you want to your subproject source.

Then, follow these steps:
- Make sure to change the name in the `Cargo.toml`.
- Leave `ffi.rs` as it is.
- Write any code you want in the `check` function in `lib.rs`.

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
