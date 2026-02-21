# Rust checks

## Structure

There are 2 folders at the root level:
- `core`: Shared code between every Rust check. This code is included in each check.
- `checks`: Folder containing every Rust-based check, each in its own crate.

## Writing Rust checks

To start writing a new Rust check, follow these steps:
- In `checks`, copy the crate `example` and rename it with your `check_name`. The check 'example' is a template for Rust-based checks.
- Change the name of the crate in `Cargo.toml`
- Add your crate in the `workspace.members` list in the `Cargo.toml` located at the root level.
- Write your implementation in the body of the `check` function in `check.rs`. 

And you're done!

Alternatively, you can create the crate with `cargo new checks/<check_name>` then copy the `Cargo.toml` and the source files manually. `cargo new` will automatically add your crate in the workspace members.

## Compiling into shared libraries

To run Rust checks in the Agent, you need to compile them into a C-Shared library with:

```
cargo build --release -p <check_name>
```

The shared library will be created in `target/release` under the name `lib<check_name>.<lib_extension>`.

## Testing shared libraries compiled from Rust checks

Now that you have 

## Additional notes

- The project is experimental. The logic and behavior of Rust-based checks may change. 
