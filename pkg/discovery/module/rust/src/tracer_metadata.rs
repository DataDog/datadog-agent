// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::fs;
use std::io::Read;
use std::path::Path;

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

use crate::language::Language;

#[derive(Debug, Serialize, Deserialize)]
pub struct TracerMetadata {
    pub schema_version: u8,
    pub runtime_id: Option<String>,
    pub tracer_language: Language,
    pub tracer_version: String,
    pub hostname: String,
    pub service_name: Option<String>,
    pub service_env: Option<String>,
    pub service_version: Option<String>,
}

/// Reads and parses tracer metadata from a process's memfd file (streaming).
pub fn get_tracer_metadata_from_path(path: &Path) -> Result<TracerMetadata> {
    const MEMFD_READ_LIMIT: u64 = 64 * 1024; // 64KB limit like datadog-agent

    let reader = fs::File::open(path).context(format!(
        "failed to open tracer memfd {}.",
        path.to_string_lossy()
    ))?;
    let reader = reader.take(MEMFD_READ_LIMIT);

    rmp_serde::from_read(reader).context(format!(
        "Failed to parse MessagePack tracer metadata for memfd {}",
        path.to_string_lossy()
    ))
}
