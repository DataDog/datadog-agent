//! Re-exports the upstream `dd-sensitive-data-scanner` C ABI so bazel can emit a
//! `cdylib`/`staticlib` named `dd_sds` for the cgo sds-go binding.
//!
//! The `#[no_mangle] extern "C"` symbols live in the `dd_sds` crate behind its
//! `dd_sds_go` feature. Re-exporting the crate root keeps them reachable so they
//! are present in this crate's static archive (which is what //deps/sds:dd_sds
//! links into the cgo binding). For the shared-library output, verify the
//! symbols survive with `nm -D bazel-bin/deps/sds/rust/libdd_sds.so`.
pub use dd_sds::*;
