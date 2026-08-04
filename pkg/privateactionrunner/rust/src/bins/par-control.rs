// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! par-control installation and executor-lifecycle scaffold.

use anyhow::{Result, bail};
use clap::Parser;
use par_control::config::Config;
use par_control::procmgr::ProcmgrLifecycle;
use std::path::PathBuf;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    #[arg(short = 'c', long, default_value = "/etc/datadog-agent/datadog.yaml")]
    config: PathBuf,
}

#[tokio::main]
async fn main() -> Result<()> {
    let result = run().await;
    log::logger().flush();
    result
}

async fn run() -> Result<()> {
    let cli = Cli::parse();
    let config = Config::from_yaml_file(&cli.config);

    if let Err(error) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: config.as_ref().map_or(log::Level::Info, |c| c.log_level),
        log_file: None,
    }) {
        eprintln!("par-control: could not initialize the logger: {error}");
    }
    let config = config?;

    if !config.split_mode {
        log::info!("private_action_runner split mode is disabled; par-control is exiting");
        return Ok(());
    }
    if cfg!(windows) {
        log::error!(
            "split mode requires the Windows named-pipe transport, which is not implemented yet; \
             par-control is exiting"
        );
        return Ok(());
    }

    let lifecycle =
        ProcmgrLifecycle::new(&config.procmgr_socket, config.executor_process_name.clone());
    lifecycle.ensure_started().await?;

    tokio::select! {
        _ = shutdown_signal() => {
            // dd-procmgrd may be synchronously waiting for par-control to exit, so
            // don't call back into it. It owns the executor and stops both processes.
            log::info!("par-control is exiting");
            Ok(())
        }
        state = lifecycle.wait_for_exit() => {
            bail!("executor exited unexpectedly with state {:?}", state?);
        }
    }
}

#[cfg(unix)]
async fn shutdown_signal() {
    use tokio::signal::unix::{SignalKind, signal};
    match signal(SignalKind::terminate()) {
        Ok(mut term) => {
            tokio::select! {
                _ = tokio::signal::ctrl_c() => {},
                _ = term.recv() => {},
            }
        }
        Err(_) => {
            let _ = tokio::signal::ctrl_c().await;
        }
    }
}

#[cfg(not(unix))]
async fn shutdown_signal() {
    let _ = tokio::signal::ctrl_c().await;
}
