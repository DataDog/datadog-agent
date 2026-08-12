// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::path::Path;

pub fn connect_lazy(path: &Path) -> tonic::transport::Channel {
    dd_procmgr_client::connect_lazy(path)
}
