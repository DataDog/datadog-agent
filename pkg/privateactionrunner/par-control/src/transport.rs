// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Local transport helpers for par-control's gRPC clients.

use std::path::Path;

/// Build a channel that connects lazily to dd-procmgrd's local endpoint.
pub fn connect_lazy(path: &Path) -> tonic::transport::Channel {
    dd_procmgr_client::connect_lazy(path)
}
