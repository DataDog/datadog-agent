// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use dd_procmgrd::config::YamlConfigLoader;
use dd_procmgrd::manager::ProcessManager;
use log::info;
use std::sync::Arc;

#[tokio::main]
async fn main() -> Result<()> {
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PROCMGR",
        level: log::Level::Info,
        log_file: None,
    })?;
    info!("dd-procmgrd starting");

    let loader = Arc::new(YamlConfigLoader::from_env());
    let mgr = ProcessManager::new(loader);
    mgr.run().await
}
