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
use log::warn;
use manager::{ExitEvent, ProcessManager};
use std::sync::Arc;
use tokio::signal::unix::{SignalKind, signal};
use tokio::sync::{mpsc, oneshot};

#[tokio::main]
async fn main() -> Result<()> {
    simple_logger::init_with_level(log::Level::Info)?;
    info!(
        "dd-procmgrd starting (version {})",
        env!("CARGO_PKG_VERSION")
    );

    let loader = Arc::new(YamlConfigLoader::from_env());
    let mgr = ProcessManager::new(loader);
    mgr.start().await;

    let (cmd_tx, mut cmd_rx) = mpsc::channel::<command::Command>(64);
    let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
    let config_path = config::config_dir().display().to_string();
    let grpc_handle = tokio::spawn(grpc::server::run(
        mgr.clone(),
        config_path,
        cmd_tx,
        grpc_shutdown_rx,
    ));

    let (exit_tx, mut exit_rx) = mpsc::channel::<ExitEvent>(256);
    let (restart_tx, mut restart_rx) = mpsc::channel::<String>(256);
    mgr.wire_watchers(&exit_tx).await;

    let mut sigterm = signal(SignalKind::terminate())?;
    let mut sigint = signal(SignalKind::interrupt())?;

    loop {
        tokio::select! {
            _ = sigterm.recv() => {
                info!("received SIGTERM");
                break;
            }
            _ = sigint.recv() => {
                info!("received SIGINT");
                break;
            }
            Some(event) = exit_rx.recv() => {
                mgr.handle_exit(event, &restart_tx).await;
            }
            Some(name) = restart_rx.recv() => {
                mgr.complete_restart(&name, &exit_tx).await;
            }
            Some(cmd) = cmd_rx.recv() => {
                match cmd {
                    command::Command::Create { name, config, reply } => {
                        let _ = reply.send(mgr.handle_create(name, *config).await);
                    }
                    command::Command::Start { name, reply } => {
                        let _ = reply.send(mgr.handle_start(&name, &exit_tx).await);
                    }
                    command::Command::Stop { name, reply } => {
                        let _ = reply.send(mgr.handle_stop(&name).await);
                    }
                    command::Command::ReloadConfig { reply } => {
                        let _ = reply.send(mgr.handle_reload_config(&exit_tx).await);
                    }
                }
            }
        }
    }

    info!("dd-procmgrd shutting down");

    let _ = grpc_shutdown_tx.send(());
    match grpc_handle.await {
        Ok(Err(e)) => warn!("gRPC server error: {e}"),
        Err(e) => warn!("gRPC server task panicked: {e}"),
        Ok(Ok(())) => {}
    }

    mgr.shutdown().await;
    info!("dd-procmgrd stopped");
    Ok(())
}
