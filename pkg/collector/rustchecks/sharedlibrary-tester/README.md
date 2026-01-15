# Shared Library tester

You can use this crate to verify that your shared library check is sending the correct payloads.
Payloads submitted by the check are printed in stdout.

This crate was inspired by @fabbing's work with his standalone crate to test his Rust implementation of `http_check`.

## Usage

To test a shared library checks, you just need to execute the following command:

```
cargo run --package sharedlibrary-tester <shared-library>
```
