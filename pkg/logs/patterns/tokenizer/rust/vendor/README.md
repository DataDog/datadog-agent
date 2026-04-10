# Vendored libpatterns Binaries

Pre-built binaries of the Rust `libpatterns` tokenization library, used by
the Go FFI bridge in `pkg/logs/patterns/tokenizer/rust/`.

## Provenance

- **Source**: `dd-source` repo, `domains/data_science/libs/rust/patterns`
- **Branch**: `yoon/rust-tokenizer-ffi` (pending merge to main)
- **Commit**: `aaafd77fb8e5b7e87da94fc2a82844dfbb719baa`
- **Built**: 2026-04-08

## Platform binaries

| Directory | Platform | File | Status |
|-----------|----------|------|--------|
| `linux_amd64/` | Linux x86_64 | `libpatterns.so` | Available |
| `linux_arm64/` | Linux aarch64 | `libpatterns.so` | Available |
| `darwin_arm64/` | macOS Apple Silicon | `libpatterns.dylib` | Available |
| `darwin_amd64/` | macOS Intel | `libpatterns.dylib` | Available |
| `windows_amd64/` | Windows x86_64 | `libpatterns.dll` | Not yet built |

If you get a linker error like `cannot find -lpatterns`, your platform's binary
is not yet vendored. Build it from dd-source (see "How to update" below) and
place it in the appropriate `vendor/<GOOS>_<GOARCH>/` directory.

## How to update

When the Rust FFI code changes in dd-source:

```bash
# macOS arm64 (from your Mac)
cd /path/to/dd-source/domains/data_science/libs/rust/patterns
cargo build --release --lib
cp ../../../../../../target/release/libpatterns.dylib \
   /path/to/datadog-agent/pkg/logs/patterns/tokenizer/rust/vendor/darwin_arm64/

# Linux amd64 (from a Linux host or Docker)
cargo build --release --lib
cp target/release/libpatterns.so \
   /path/to/datadog-agent/pkg/logs/patterns/tokenizer/rust/vendor/linux_amd64/

# Header (platform-independent)
cbindgen --output /path/to/datadog-agent/pkg/logs/patterns/tokenizer/rust/vendor/include/libpatterns.h
```

## Temporary

This vendoring approach is temporary. Once the dd-source PR merges:
1. A CI job will build and upload binaries
2. The agent build will download them instead of using vendored copies
3. This `vendor/` directory will be removed
