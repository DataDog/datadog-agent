// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Process-manager daemon entry point.
//!
//! On Windows this binary doubles as the SCM service: it calls
//! `StartServiceCtrlDispatcherW` first and, if SCM is not the launcher,
//! falls back to console mode. On Unix it runs as a plain console daemon.

fn main() {
    let result = run();
    if let Err(e) = result {
        eprintln!("dd-procmgrd failed: {e:#}");
        std::process::exit(1);
    }
}

#[cfg(windows)]
fn run() -> anyhow::Result<()> {
    dd_procmgrd::service::run_as_service()
}

#[cfg(not(windows))]
fn run() -> anyhow::Result<()> {
    use dd_procmgrd::config::YamlConfigLoader;
    use dd_procmgrd::manager::ProcessManager;
    use dd_procmgrd::uuid_gen::V4UuidGenerator;
    use log::info;
    use std::sync::Arc;

    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PROCMGR",
        level: log::Level::Info,
        log_file: None,
    })?;
    info!("dd-procmgrd starting");

    let runtime = tokio::runtime::Runtime::new()?;
    runtime.block_on(async {
        let loader = Arc::new(YamlConfigLoader::from_env());
        let mgr = ProcessManager::new(loader, Arc::new(V4UuidGenerator));
        mgr.run().await
    })
}
