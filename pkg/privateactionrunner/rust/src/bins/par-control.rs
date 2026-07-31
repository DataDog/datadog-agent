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

    /// Command (argv) to run the Go one-shot enroll when no identity is persisted
    /// yet, e.g. `--enroll-command privateactionrunner rotate-identity --cfgpath
    /// /etc/datadog-agent/datadog.yaml`. Consumes every remaining argument,
    /// including ones starting with `-` (the enrolled command has its own flags),
    /// so it must be the last option on the command line. If omitted, an existing
    /// identity is required.
    #[arg(long = "enroll-command", num_args = 1.., allow_hyphen_values = true)]
    enroll_command: Vec<String>,
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();

    // Wire the shared agent logger (the one dd-procmgrd also uses) before doing
    // anything else, so the `log` macros throughout the orchestration loop
    // actually emit. It writes to stdout, which the process manager inherits
    // (`stdout: inherit` in the process definition).
    //
    // The level comes from the agent's own `log_level` key, read straight from
    // the YAML: this runs before the config/identity load so that a stand-down
    // decision or a config error is itself logged. A logger init failure is not
    // fatal — a control plane running without logs beats one refusing to run.
    if let Err(e) = dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "PAR-CONTROL",
        level: log_level_from_yaml_file(&cli.config),
        log_file: None,
    }) {
        eprintln!("par-control: could not initialize the logger: {e}");
    }

    // The procmgr definition that starts par-control is installed unconditionally,
    // so this process decides for itself whether the split deployment is active.
    // Exit 0 (not an error) when it is not: `restart: on-failure` then leaves the
    // process alone instead of hitting the restart limit on every host that has
    // never opted in.
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
        // par-control's local transport is Unix-socket only (see transport.rs).
        // On Windows it can reach OPMS but can neither start nor dispatch to the
        // executor, so it would dequeue real tasks and fail every one of them.
        // Refuse up front instead, and exit 0 so procmgr does not restart-loop.
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
    // Secure the control<->executor channel with mTLS via the agent IPC cert. The
    // cert is read on each connection, not now: it may not exist yet (see
    // transport::connect_lazy_tls).
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
