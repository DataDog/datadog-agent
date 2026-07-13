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
    /// Path to the agent datadog.yaml.
    #[arg(short = 'c', long, default_value = "/etc/datadog-agent/datadog.yaml")]
    config: PathBuf,

    /// Command (argv) to run the Go one-shot enroll when no identity is persisted
    /// yet, e.g. `--enroll-command privateactionrunner --enroll-command enroll
    /// --enroll-command -c --enroll-command /etc/datadog-agent`. If omitted, an
    /// existing identity is required.
    #[arg(long = "enroll-command", num_args = 1..)]
    enroll_command: Vec<String>,
}

#[tokio::main]
async fn main() -> Result<()> {
    // NOTE: a logger implementation (e.g. dd-agent-log) should be initialized
    // here so the `log` macros emit; left unwired in this first cut.
    let cli = Cli::parse();

    // Ensure the runner has an identity, triggering the Go one-shot enroll if not.
    let config = bootstrap::load_config_with_bootstrap(&cli.config, &cli.enroll_command)?;

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
    ));
    let lifecycle = Arc::new(ProcmgrLifecycle::new(
        &config.procmgr_socket,
        config.executor_process_name.clone(),
    ));
    // Secure the control<->executor channel with mTLS via the agent IPC cert.
    let executor_tls = match &config.ipc_cert_file {
        Some(path) => Some(par_control::tls::build_ipc_client_connector(path)?),
        None => {
            log::warn!(
                "ipc_cert_file_path is not set; dispatching to the executor without mTLS \
                 (the executor requires a client cert, so this will fail in production)"
            );
            None
        }
    };
    let dispatcher = Arc::new(ExecutorDispatcher::new(&config.executor_socket, executor_tls));

    let params = Params::from_config(&config);
    let orchestrator = Orchestrator::new(opms, lifecycle, dispatcher, params);

    orchestrator.run(shutdown_signal()).await;
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
