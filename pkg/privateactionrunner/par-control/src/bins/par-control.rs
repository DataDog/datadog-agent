// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use clap::Parser;
use par_control::bootstrap;
use par_control::config::Launch;
use par_control::executor::ExecutorDispatcher;
use par_control::jwt::{Es256Signer, JwtSigner};
use par_control::opms::{HttpOpms, HttpOpmsConfig};
use par_control::orchestrator::{Orchestrator, Params};
use par_control::platform;
use par_control::procmgr::ProcmgrLifecycle;
use std::path::PathBuf;
use std::process::ExitCode;
use std::sync::Arc;

#[derive(Parser)]
#[command(name = "par-control", about = "Private Action Runner control plane")]
struct Cli {
    #[arg(short = 'c', long, default_value_os_t = platform::default_config_path())]
    config: PathBuf,

    #[arg(
        long = "ensure-enrollment-command",
        num_args = 1..,
        allow_hyphen_values = true
    )]
    ensure_enrollment_command: Vec<String>,
}

#[tokio::main]
async fn main() -> ExitCode {
    // Return ExitCode so main does not print the error a second time.
    let code = match run().await {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            log::error!("par-control failed: {error:#}");
            ExitCode::FAILURE
        }
    };
    log::logger().flush();
    code
}

async fn run() -> Result<()> {
    let cli = Cli::parse();
    // Resolve the log level first so a config error is logged at that level.
    // A logging failure does not prevent the runner from starting.
    let launch = Launch::from_yaml_file(&cli.config);
    let log_level = launch
        .as_ref()
        .map_or(log::LevelFilter::Info, |launch| launch.log_level);
    if let Err(e) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: log_level.to_level().unwrap_or(log::Level::Error),
        // dd-procmgrd redirects stdout and stderr per its process definition.
        log_file: None,
    }) {
        eprintln!("par-control: could not initialize the logger: {e}");
    }
    log::set_max_level(log_level);

    if !launch?.gate.split_mode {
        log::info!(
            "private_action_runner.split_enabled is not enabled; \
             the monolithic runner owns OPMS polling. Exiting."
        );
        return Ok(());
    }

    let config =
        bootstrap::load_config_with_bootstrap(&cli.config, &cli.ensure_enrollment_command)?;

    let signer: Arc<dyn JwtSigner> = Arc::new(Es256Signer::new(
        config.identity.org_id,
        config.identity.runner_id.clone(),
        &config.identity.private_key,
    )?);

    let opms = Arc::new(HttpOpms::new(
        config.opms_base_url.clone(),
        signer,
        HttpOpmsConfig {
            runner_version: config.runner_version.clone(),
            modes: config.modes.clone(),
            timeout: config.opms_request_timeout,
            proxy: config.proxy.clone(),
            tls: config.tls.clone(),
            extra_headers: config.opms_extra_headers.clone(),
        },
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

/// Handle CTRL_BREAK, which dd-procmgrd uses for graceful Windows stops.
#[cfg(windows)]
async fn shutdown_signal() {
    match tokio::signal::windows::ctrl_break() {
        Ok(mut ctrl_break) => {
            tokio::select! {
                _ = tokio::signal::ctrl_c() => {},
                _ = ctrl_break.recv() => {},
            }
        }
        Err(error) => {
            log::warn!("could not listen for CTRL_BREAK, falling back to CTRL_C: {error}");
            let _ = tokio::signal::ctrl_c().await;
        }
    }
}
