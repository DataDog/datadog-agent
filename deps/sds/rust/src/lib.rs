//! Re-exports the upstream `dd-sensitive-data-scanner` C ABI (its `dd_sds_go`
//! FFI symbols) so bazel can emit a `staticlib` named `dd_sds` for the cgo
//! sds-go binding (linked via //deps/sds:dd_sds).
pub use dd_sds::*;
