//! Re-exports the `dd-sensitive-data-scanner` C ABI (its `dd_sds_go` FFI symbols)
//! so bazel can emit a `cdylib`/`staticlib` named `dd_sds` for the cgo sds-go
//! binding (linked via //deps/sds:dd_sds).
pub use dd_sds::*;
