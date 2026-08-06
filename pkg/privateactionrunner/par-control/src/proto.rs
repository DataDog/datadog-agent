// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Generated protobuf/gRPC bindings for dd-procmgrd.

#[cfg(not(bazel))]
pub mod procmgr {
    tonic::include_proto!("datadog.procmgr");
}

#[cfg(bazel)]
pub mod procmgr {
    pub use procmgr_proto::datadog::procmgr::*;
}
