// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Managed-process spawn: config → [`SpawnRequest`] → platform backend.

mod profile;
mod request;
mod stdio;

#[cfg(windows)]
pub(crate) use profile::DATADOG_AGENT_PROCESS;
pub use profile::{SpawnProfile, profile_for};
pub use request::SpawnRequest;

use anyhow::Result;

use crate::config::ProcessConfig;
use crate::handle::ProcessHandle;
use crate::platform;

/// Resolve spawn profile and inputs from config, then delegate to the platform backend.
pub(crate) fn spawn_managed_child(
    process_name: &str,
    config: &ProcessConfig,
) -> Result<ProcessHandle> {
    let request = SpawnRequest::from_config(process_name, config)?;
    platform::spawn_child(process_name, request, profile_for(process_name))
}
