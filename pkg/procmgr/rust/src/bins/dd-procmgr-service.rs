// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows service entry point for dd-procmgrd.
//!
//! This binary is functionally identical to `dd-procmgrd` but is named
//! `dd-procmgr-service` for the Windows installer. A separate source file
//! avoids the Cargo warning about a shared source across multiple `[[bin]]`
//! targets.

use anyhow::Result;
use dd_procmgrd::config::YamlConfigLoader;
use dd_procmgrd::manager::ProcessManager;
use dd_procmgrd::uuid_gen::V4UuidGenerator;
use log::info;
use std::sync::Arc;

#[tokio::main]
async fn main() -> Result<()> {
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PROCMGR",
        level: log::Level::Info,
        log_file: None,
    })?;
    info!("dd-procmgr-service starting");

    let loader = Arc::new(YamlConfigLoader::from_env());
    let mgr = ProcessManager::new(loader, Arc::new(V4UuidGenerator));
    mgr.run().await
}
