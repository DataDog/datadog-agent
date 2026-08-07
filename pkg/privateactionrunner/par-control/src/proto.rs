// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Protobuf/gRPC bindings for services par-control talks to.
//!
//! Process-manager bindings come from the shared client crate. Bazel generates
//! executor bindings next to the canonical proto, while Cargo uses `build.rs`.

/// Process-manager service (`datadog.procmgr`).
pub mod procmgr {
    pub use dd_procmgr_client::proto::*;
}

/// Shared action error type imported by the executor protocol.
#[cfg(not(bazel))]
pub mod errorcode {
    tonic::include_proto!("datadog.privateactionrunner.errorcode");
}

/// Control<->executor service (`datadog.privateactionrunner.executor`).
#[cfg(not(bazel))]
pub mod executor {
    tonic::include_proto!("datadog.privateactionrunner.executor");
}

#[cfg(bazel)]
pub mod executor {
    pub use executor_proto::datadog::privateactionrunner::executor::*;
}
