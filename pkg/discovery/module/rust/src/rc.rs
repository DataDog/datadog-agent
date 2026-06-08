// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Generated gRPC bindings for the minimal AgentSecure remote-config subset.
//!
//! The modules are nested to match the proto package paths (datadog.config and
//! datadog.api.v1) so prost's cross-package references resolve. This is
//! generated code, so the crate's strict lints are relaxed here.
#![allow(
    clippy::all,
    clippy::pedantic,
    clippy::nursery,
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::string_slice,
    clippy::cast_possible_wrap,
    clippy::undocumented_unsafe_blocks,
    dead_code,
    unused_imports,
    missing_docs
)]

pub mod datadog {
    pub mod config {
        include!(concat!(env!("OUT_DIR"), "/datadog.config.rs"));
    }
    pub mod api {
        pub mod v1 {
            include!(concat!(env!("OUT_DIR"), "/datadog.api.v1.rs"));
        }
    }
}
