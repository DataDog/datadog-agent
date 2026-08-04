// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Generated protobuf/gRPC bindings for the two services par-control talks to:
//! the process manager (`dd-procmgrd`) and the on-demand executor.
//!
//! As in the procmgr crate, Bazel builds consume the `rust_prost_library` crates
//! generated next to the canonical `.proto` files, while `cargo`/IDE builds fall
//! back to `tonic::include_proto!` from the OUT_DIR produced by `build.rs`. The
//! `--cfg=bazel` rustc flag (set in BUILD.bazel) selects the Bazel path.

/// Process-manager service (`datadog.procmgr`).
#[cfg(not(bazel))]
pub mod procmgr {
    tonic::include_proto!("datadog.procmgr");
}

#[cfg(bazel)]
pub mod procmgr {
    // Crate name from //pkg/proto/datadog/procmgr:procmgr_rust_proto.
    pub use procmgr_proto::datadog::procmgr::*;
}

/// Control<->executor service (`datadog.privateactionrunner.executor`). The
/// embedded `ActionPlatformError` (from the imported error_code.proto) is reached
/// through the generated `ActionResult` value, so no separate module is needed here.
#[cfg(not(bazel))]
pub mod executor {
    tonic::include_proto!("datadog.privateactionrunner.executor");
}

#[cfg(bazel)]
pub mod executor {
    // Crate name from //pkg/proto/datadog/privateactionrunner:executor_rust_proto.
    pub use executor_proto::datadog::privateactionrunner::executor::*;
}
