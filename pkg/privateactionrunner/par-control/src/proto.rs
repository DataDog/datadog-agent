// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Protobuf/gRPC bindings for services par-control talks to.

/// Process-manager service (`datadog.procmgr`).
pub mod procmgr {
    pub use dd_procmgr_client::proto::*;
}
