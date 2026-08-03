// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! `par-control` binary: the always-on Private Action Runner control plane.
//! Loads the runner identity/config, wires the OPMS client, process-manager
//! lifecycle, and executor dispatcher, and runs the orchestration loop until a
//! termination signal.

use anyhow::Result;
use clap::Parser;
use par_control::bootstrap;
use par_control::config::{LaunchGate, log_level_from_yaml_file};
use par_control::executor::ExecutorDispatcher;
use par_control::jwt::{Es256Signer, JwtSigner};
use par_control::opms::HttpOpms;
use par_control::orchestrator::{Orchestrator, Params};
use par_control::procmgr::ProcmgrLifecycle;
use std::path::PathBuf;
use std::sync::Arc;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    #[arg(short = 'c', long, default_value = "/etc/datadog-agent/datadog.yaml")]
    config: PathBuf,

    /// Go one-shot enrollment command. Must be the last option because it
    /// consumes all remaining arguments.
    #[arg(long = "enroll-command", num_args = 1.., allow_hyphen_values = true)]
    enroll_command: Vec<String>,
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();

    // Initialize logging before the launch gate so clean exits and config errors
    // are visible. Logging failure does not prevent the runner from starting.
    if let Err(e) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: log_level_from_yaml_file(&cli.config),
        log_file: None,
    }) {
        eprintln!("par-control: could not initialize the logger: {e}");
    }

    // The process definition is installed unconditionally; inactive hosts exit 0.
    let gate = LaunchGate::from_yaml_file(&cli.config)?;
    if !gate.split_mode {
        log::info!(
            "private_action_runner.split_enabled is not enabled; \
             the monolithic runner owns OPMS polling. Exiting."
        );
        log::logger().flush();
        return Ok(());
    }
    if cfg!(windows) {
        // Named-pipe transport is not implemented. The Go monolith remains active
        // on Windows even when split_enabled is set.
        log::error!(
            "the split deployment model is not supported on Windows yet \
             (named-pipe transport to dd-procmgrd and the executor is not implemented); \
             unset private_action_runner.split_enabled. Exiting."
        );
        log::logger().flush();
        return Ok(());
    }

    let config =
        bootstrap::load_config_with_bootstrap(&cli.config, &cli.enroll_command, gate.self_enroll)?;

    let signer: Arc<dyn JwtSigner> = Arc::new(Es256Signer::new(
        config.identity.org_id,
        config.identity.runner_id.clone(),
        &config.identity.private_key,
    )?);

    let opms = Arc::new(HttpOpms::new(
        config.opms_base_url.clone(),
        signer,
        config.runner_version.clone(),
        config.modes.clone(),
        config.opms_request_timeout,
    )?);
    let lifecycle = Arc::new(ProcmgrLifecycle::new(
        &config.procmgr_socket,
        config.executor_process_name.clone(),
    ));
    // The IPC certificate is loaded lazily because the executor may create it.
    let dispatcher = Arc::new(ExecutorDispatcher::new(
        &config.executor_socket,
        Some(&config.ipc_cert_file),
    ));

    let params = Params::from_config(&config);
    let orchestrator = Orchestrator::new(opms, lifecycle, dispatcher, params);

    log::info!(
        "par-control starting: version={} urn={} opms={} executor_socket={} procmgr_socket={} ipc_cert={}",
        config.runner_version,
        config.identity.urn,
        config.opms_base_url,
        config.executor_socket.display(),
        config.procmgr_socket.display(),
        config.ipc_cert_file.display(),
    );

    orchestrator.run(shutdown_signal()).await;
    log::info!("par-control stopped");
    log::logger().flush();
    Ok(())
}

/// Resolves when the process receives Ctrl-C or (on Unix) SIGTERM.
async fn shutdown_signal() {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{SignalKind, signal};
        let mut term = match signal(SignalKind::terminate()) {
            Ok(s) => s,
            Err(_) => {
                let _ = tokio::signal::ctrl_c().await;
                return;
            }
        };
        tokio::select! {
            _ = tokio::signal::ctrl_c() => {},
            _ = term.recv() => {},
        }
    }
    #[cfg(not(unix))]
    {
        let _ = tokio::signal::ctrl_c().await;
    }
}
