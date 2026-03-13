// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod command;
mod config;
mod env;
mod grpc;
mod manager;
mod ordering;
mod process;
mod shutdown;
mod state;

use anyhow::Result;
use config::YamlConfigLoader;
use log::info;
use manager::ProcessManager;
use std::sync::Arc;

#[tokio::main]
async fn main() -> Result<()> {
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PROCMGR",
        level: log::Level::Info,
        log_file: None,
    })?;
    info!(
        "dd-procmgrd starting (version {})",
        env!("CARGO_PKG_VERSION")
    );

    let loader = Arc::new(YamlConfigLoader::from_env());
    let mgr = ProcessManager::new(loader);
    mgr.run().await
}
