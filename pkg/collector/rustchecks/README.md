# Rust checks

## Structure

There are 2 folders at the root level:
- `core`: Shared code between every Rust check. This code is included in each check.
- `checks`: Folder containing every Rust-based check, each in its own project.

## Writing a Rust check

To start writing a new Rust check, you have 2 options:
- Make a copy of `example` in `checks`, which is an "Hello world" Rust check that you can use as a template.
- Create a new Rust project in `checks` and copy `lib.rs` from any checks you want.

Then, follow these steps:
- In `Cargo.toml`, change the name and make sure it has a property named `lib`.
- In `lib.rs`, change the version (const `VERSION` value).
- Write any code you want in the `check` function in `lib.rs`.

And you're done!

## Building a Rust check

Building a Rust check means compiling it into a C-Shared library.

To compile a Rust check into a shared library, you can use the following command:
```
cargo build --release -p <check_name>
```

You will then find the shared library in `target/release` under the name `lib<check_name>.<lib_extension>`.

## Additional notes

- The project is experimental. The logic and behavior of Rust-based checks may change. 
- `httpcheck` should be refactored.
