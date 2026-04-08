// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::path::{Path, PathBuf};

const DEFAULT_PIPE_PATH: &str = r"\\.\pipe\datadog-procmgrd";

/// Placeholder URI for tonic Endpoint when connecting over Named Pipes.
pub const DUMMY_ENDPOINT: &str = "http://[::]:50051";

pub fn ipc_path() -> PathBuf {
    std::env::var("DD_PM_SOCKET_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from(DEFAULT_PIPE_PATH))
}

pub fn prepare(_path: &Path) -> Result<()> {
    Ok(())
}

pub fn set_permissions(_path: &Path) {}

pub fn cleanup(_path: &Path) {}
