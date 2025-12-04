//! Process Manager Daemon
//!
//! This daemon provides gRPC and REST APIs for managing system processes.
//!
//! Platform support:
//! - Linux/macOS: Unix socket (default) or TCP transports
//! - Windows: TCP transport (default, Unix sockets not available)
//!
//! Configuration is loaded from environment variables (no CLI arguments).

mod daemon {
    pub mod config;
}
use daemon::config::{DaemonConfig, TransportMode};
use pm_engine::{
    adapters::{
        grpc::{serve_on_unix_socket_with_health, ProcessManagerService},
        rest::{build_router, serve_on_unix_socket as rest_unix},
    },
    application::Application,
    domain::ports::ProcessRepository,
    domain::services::{
        HealthMonitoringService, ProcessSupervisionService, ProcessWatchingService,
        SocketActivationService,
    },
    domain::LoadConfigCommand,
    domain::SocketConfig as DomainSocketConfig,
    infrastructure::{
        get_default_config_path, InMemoryProcessRepository, StandardHealthCheckExecutor,
        TokioProcessExecutor,
    },
    proto::process_manager::process_manager_server::ProcessManagerServer,
};
use std::sync::Arc;
use std::time::SystemTime;
use tokio_util::sync::CancellationToken;
use tonic::transport::Server as TonicServer;
use tonic_health::server::health_reporter;
use tracing::{debug, error, info, warn};
use tracing_subscriber::EnvFilter;

// Platform-specific signal handling imports
#[cfg(unix)]
use tokio::signal::unix::{signal as unix_signal, SignalKind};

/// Wait for shutdown signal
/// - Unix: SIGINT or SIGTERM
/// - Windows: Ctrl+C
#[cfg(unix)]
async fn wait_for_shutdown_signal() -> &'static str {
    let mut sigterm =
        unix_signal(SignalKind::terminate()).expect("Failed to install SIGTERM handler");
    let mut sigint =
        unix_signal(SignalKind::interrupt()).expect("Failed to install SIGINT handler");

    tokio::select! {
        _ = sigterm.recv() => "SIGTERM",
        _ = sigint.recv() => "SIGINT",
    }
}

#[cfg(windows)]
async fn wait_for_shutdown_signal() -> &'static str {
    tokio::signal::ctrl_c()
        .await
        .expect("Failed to install Ctrl+C handler");
    "Ctrl+C"
}

#[cfg(not(any(unix, windows)))]
async fn wait_for_shutdown_signal() -> &'static str {
    // Fallback: just wait forever (would need platform-specific implementation)
    std::future::pending::<()>().await;
    "Unknown"
}

/// Graceful shutdown: stop all managed processes
async fn graceful_shutdown(registry: &Application) {
    info!("Starting graceful shutdown of managed processes...");

    // Get list of all processes
    let list_result = registry.list_processes().execute().await;

    match list_result {
        Ok(response) => {
            let running_count = response.processes.iter().filter(|p| p.is_running()).count();
            info!(running = running_count, "Stopping managed processes");

            for process in response.processes {
                if process.is_running() {
                    info!(process = %process.name(), pid = ?process.pid(), "Stopping process");
                    let stop_cmd = pm_engine::domain::StopProcessCommand::from_name(
                        process.name().to_string(),
                    );
                    if let Err(e) = registry.stop_process().execute(stop_cmd).await {
                        warn!(process = %process.name(), error = %e, "Failed to stop process during shutdown");
                    }
                }
            }
        }
        Err(e) => {
            error!(error = %e, "Failed to list processes during shutdown");
        }
    }

    info!("Graceful shutdown complete");
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Load configuration from environment variables
    let config = DaemonConfig::from_env();
    config.validate()?;

    // Initialize logging with configured level
    let env_filter = EnvFilter::new(&config.log_level);
    tracing_subscriber::fmt()
        .with_env_filter(env_filter)
        .with_target(false)
        .init();

    info!("Starting Process Manager Daemon");
    info!(
        transport = ?config.transport_mode,
        grpc_port = config.grpc_port,
        rest_port = config.rest_port,
        grpc_socket = %config.grpc_socket,
        rest_socket = %config.rest_socket,
        enable_rest = config.enable_rest,
        "Daemon configuration loaded from environment"
    );

    // Track startup time for uptime calculation
    let _startup_time = SystemTime::now();

    // 1. Setup Infrastructure Layer (Driven Adapters)
    let repository: Arc<dyn ProcessRepository> = Arc::new(InMemoryProcessRepository::new());
    // Keep concrete type to satisfy ResourceUsageReader + ProcessExecutor bound
    let executor = Arc::new(TokioProcessExecutor::new());
    info!("Infrastructure layer initialized");

    // 2. Setup Process Monitoring (Event-Driven)
    let (watcher, exit_rx) = ProcessWatchingService::new();
    let watcher = Arc::new(watcher);

    // 2.5. Setup Health Monitor
    // Health monitor detects unhealthy processes and stops them
    // ProcessSupervisionService coordinates when to start monitoring
    let health_executor = Arc::new(StandardHealthCheckExecutor::new());
    let health_monitor = Arc::new(HealthMonitoringService::new(
        repository.clone(),
        health_executor,
        executor.clone(),
    ));
    info!("Health monitor initialized");

    // 2.75. Setup Process Supervisor (Single Coordinator for all lifecycle management)
    let supervisor = Arc::new(ProcessSupervisionService::with_watcher_and_health_monitor(
        repository.clone(),
        executor.clone(),
        watcher,
        health_monitor.clone(),
    ));

    // Start supervisor in background task
    let cancellation_token = CancellationToken::new();
    let supervisor_token = cancellation_token.clone();
    let supervisor_for_run = supervisor.clone();
    tokio::spawn(async move {
        supervisor_for_run.run(exit_rx, supervisor_token).await;
    });
    info!("Process supervisor started (single coordinator: exit + health + restart)");

    // 2.8. Setup Socket Activation Manager
    let (socket_manager, socket_rx) = SocketActivationService::new();
    let socket_manager = Arc::new(socket_manager);
    info!("Socket activation manager initialized");

    // 3. Setup Application Layer (KISS: just pass supervisor)
    let registry = Arc::new(Application::new_with_supervisor(
        repository.clone(),
        executor.clone(),
        supervisor.clone(),
    ));
    info!("Application layer initialized (9 use cases with supervisor coordination)");

    // 2.5. Load configuration if available (directory-based only)
    let config_path = match &config.config_dir {
        Some(path) => {
            info!(config_dir = %path, "Using config from DD_PM_CONFIG_DIR");
            Some(path.clone())
        }
        None => get_default_config_path(),
    };

    if let Some(ref path) = config_path {
        info!(config_path = %path, "Loading configuration");
        let load_cmd = LoadConfigCommand {
            config_path: path.clone(),
        };

        match registry.load_config().execute(load_cmd).await {
            Ok(result) => {
                info!(
                    created = result.processes_created,
                    started = result.processes_started,
                    errors = result.errors.len(),
                    "Configuration loaded successfully"
                );

                if !result.errors.is_empty() {
                    for error in &result.errors {
                        error!("Config error: {}", error);
                    }
                }
                // LoadConfig use case handles all registration internally via supervisor
            }
            Err(e) => {
                error!(error = %e, "Failed to load configuration");
                // Continue anyway - daemon should still start
            }
        }
        // Load sockets from config
        load_sockets_from_config(config_path.as_ref().unwrap(), socket_manager.clone()).await;
    } else {
        info!(
            "No configuration directory found. \
            Set DD_PM_CONFIG_DIR or create /etc/datadog-agent/process-manager/processes.d/ to auto-load processes."
        );
    }

    // 2.75. Start socket activation event handler
    // This task listens for socket activation events and automatically starts processes
    let socket_registry = registry.clone();
    tokio::spawn(async move {
        handle_socket_activation_events(socket_rx, socket_registry).await;
    });
    info!("Socket activation event handler started");

    // 3. Setup Driving Adapters
    if config.transport_mode == TransportMode::Tcp {
        // TCP mode: Use TCP for primary transport
        info!("Starting in TCP mode");

        // On Windows, this is the only supported mode
        #[cfg(windows)]
        {
            let grpc_addr: std::net::SocketAddr =
                format!("127.0.0.1:{}", config.grpc_port).parse()?;
            let grpc_service = ProcessManagerService::new(registry.clone());

            // Setup health checking
            let (mut health_reporter, health_service) = health_reporter();
            health_reporter
                .set_serving::<ProcessManagerServer<ProcessManagerService>>()
                .await;

            info!("gRPC server listening on TCP {}", grpc_addr);
            info!("Health check service enabled");

            if config.enable_rest {
                let rest_addr: std::net::SocketAddr =
                    format!("127.0.0.1:{}", config.rest_port).parse()?;
                let rest_app = build_router(registry.clone());
                let rest_server =
                    axum::Server::bind(&rest_addr).serve(rest_app.into_make_service());

                info!("REST server listening on http://{}", rest_addr);
                info!("Daemon ready. All services started");

                let grpc_tcp_server = TonicServer::builder()
                    .add_service(health_service)
                    .add_service(ProcessManagerServer::new(grpc_service))
                    .serve(grpc_addr);

                tokio::select! {
                    result = grpc_tcp_server => {
                        if let Err(e) = result {
                            error!("gRPC TCP server error: {}", e);
                        }
                    }
                    result = rest_server => {
                        if let Err(e) = result {
                            error!("REST server error: {}", e);
                        }
                    }
                    signal_name = wait_for_shutdown_signal() => {
                        info!(signal = signal_name, "Received shutdown signal");
                    }
                }
            } else {
                info!("Daemon ready. All services started");

                let grpc_tcp_server = TonicServer::builder()
                    .add_service(health_service)
                    .add_service(ProcessManagerServer::new(grpc_service))
                    .serve(grpc_addr);

                tokio::select! {
                    result = grpc_tcp_server => {
                        if let Err(e) = result {
                            error!("gRPC TCP server error: {}", e);
                        }
                    }
                    signal_name = wait_for_shutdown_signal() => {
                        info!(signal = signal_name, "Received shutdown signal");
                    }
                }
            }

            // Cancel supervisor
            cancellation_token.cancel();

            // Graceful shutdown
            graceful_shutdown(&registry).await;
        }

        // On Unix, TCP mode also includes Unix socket for backward compatibility
        #[cfg(unix)]
        {
            let grpc_addr = format!("0.0.0.0:{}", config.grpc_port).parse()?;
            let grpc_tcp_service = ProcessManagerService::new(registry.clone());
            let grpc_unix_service = ProcessManagerService::new(registry.clone());
            let grpc_socket_path = config.grpc_socket.clone();

            // Setup standard gRPC health checking for TCP
            let (mut health_reporter, health_service) = health_reporter();
            health_reporter
                .set_serving::<ProcessManagerServer<ProcessManagerService>>()
                .await;

            // TCP server
            let grpc_tcp_server = TonicServer::builder()
                .add_service(health_service)
                .add_service(ProcessManagerServer::new(grpc_tcp_service))
                .serve(grpc_addr);

            info!("gRPC server listening on {} (TCP)", grpc_addr);

            // Setup health checking for Unix socket
            let (mut unix_health_reporter, unix_health_service) =
                tonic_health::server::health_reporter();
            unix_health_reporter
                .set_serving::<ProcessManagerServer<ProcessManagerService>>()
                .await;

            // Unix socket server
            let grpc_unix_server = async move {
                if let Err(e) = serve_on_unix_socket_with_health(
                    &grpc_socket_path,
                    grpc_unix_service,
                    unix_health_service,
                )
                .await
                {
                    error!("Unix socket server error: {}", e);
                }
            };

            info!(
                "gRPC server listening on {} (Unix socket)",
                config.grpc_socket
            );
            info!("Health check service enabled");

            if config.enable_rest {
                let rest_addr = format!("0.0.0.0:{}", config.rest_port);
                let rest_app = build_router(registry.clone());
                let rest_server =
                    axum::Server::bind(&rest_addr.parse()?).serve(rest_app.into_make_service());

                info!("REST server listening on http://{}", rest_addr);
                info!("Daemon ready. All services started");

                // Run all three servers (TCP, Unix socket, REST)
                tokio::select! {
                    result = grpc_tcp_server => {
                        if let Err(e) = result {
                            error!("gRPC TCP server error: {}", e);
                        }
                    }
                    _ = grpc_unix_server => {
                        // Unix server task completed
                    }
                    result = rest_server => {
                        if let Err(e) = result {
                            error!("REST server error: {}", e);
                        }
                    }
                    signal_name = wait_for_shutdown_signal() => {
                        info!(signal = signal_name, "Received shutdown signal");
                    }
                }

                // Cancel supervisor first
                cancellation_token.cancel();

                // Graceful shutdown: stop all managed processes
                graceful_shutdown(&registry).await;
            } else {
                info!("Daemon ready. All services started");

                // Run both gRPC servers (TCP + Unix socket)
                tokio::select! {
                    result = grpc_tcp_server => {
                        if let Err(e) = result {
                            error!("gRPC TCP server error: {}", e);
                        }
                    }
                    _ = grpc_unix_server => {
                        // Unix server task completed
                    }
                    signal_name = wait_for_shutdown_signal() => {
                        info!(signal = signal_name, "Received shutdown signal");
                    }
                }

                // Cancel supervisor first
                cancellation_token.cancel();

                // Graceful shutdown: stop all managed processes
                graceful_shutdown(&registry).await;
            }
        }
    } else {
        // Unix socket mode (default on Unix)
        // On Windows, this branch should not be reached due to default TransportMode::Tcp

        #[cfg(windows)]
        {
            // On Windows, Unix socket mode is not available
            // The config should default to TCP mode, but if somehow we get here, error out
            error!(
                "Unix socket mode is not supported on Windows. \
                Set DD_PM_TRANSPORT_MODE=tcp or use the default configuration."
            );
            return Err("Unix sockets not supported on Windows".into());
        }

        #[cfg(unix)]
        {
            info!("Starting in Unix socket mode");

            let grpc_service = ProcessManagerService::new(registry.clone());
            let grpc_socket_path = config.grpc_socket.clone();

            // Setup standard gRPC health checking
            let (mut health_reporter, health_service) = health_reporter();
            health_reporter
                .set_serving::<ProcessManagerServer<ProcessManagerService>>()
                .await;

            let grpc_server = async move {
                if let Err(e) = serve_on_unix_socket_with_health(
                    &grpc_socket_path,
                    grpc_service,
                    health_service,
                )
                .await
                {
                    error!("gRPC server error: {}", e);
                }
            };

            info!("gRPC server listening on {}", config.grpc_socket);
            info!("Health check service enabled");

            if config.enable_rest {
                let rest_app = build_router(registry.clone());
                let rest_socket_path = config.rest_socket.clone();

                let rest_server = async move {
                    if let Err(e) = rest_unix(&rest_socket_path, rest_app).await {
                        error!("REST server error: {}", e);
                    }
                };

                info!("REST server listening on {}", config.rest_socket);
                info!("Daemon ready. All services started");

                // Run both servers
                tokio::select! {
                    _ = grpc_server => {}
                    _ = rest_server => {}
                    signal_name = wait_for_shutdown_signal() => {
                        info!(signal = signal_name, "Received shutdown signal");
                    }
                }
            } else {
                info!("Daemon ready. All services started");

                tokio::select! {
                    _ = grpc_server => {}
                    signal_name = wait_for_shutdown_signal() => {
                        info!(signal = signal_name, "Received shutdown signal");
                    }
                }
            }

            // Cancel supervisor first
            cancellation_token.cancel();

            // Graceful shutdown: stop all managed processes
            graceful_shutdown(&registry).await;

            // Cleanup sockets (Unix only)
            let _ = std::fs::remove_file(&config.grpc_socket);
            if config.enable_rest {
                let _ = std::fs::remove_file(&config.rest_socket);
            }
        }

        #[cfg(not(any(unix, windows)))]
        {
            error!("This platform is not supported");
            return Err("Platform not supported".into());
        }
    }

    info!("Process Manager Daemon stopped");
    Ok(())
}

/// Handle socket activation events and automatically start processes
async fn handle_socket_activation_events(
    mut socket_rx: tokio::sync::mpsc::UnboundedReceiver<
        pm_engine::domain::services::SocketActivationEvent,
    >,
    registry: Arc<Application>,
) {
    use pm_engine::domain::StartProcessCommand;

    info!("Socket activation event handler running");

    // Get services/use cases we need
    let spawn_service = registry.spawn_service();
    let start_use_case = registry.start_process();

    while let Some(event) = socket_rx.recv().await {
        info!(
            socket = %event.socket_name,
            service = %event.service_name,
            fd = event.fd,
            accept = event.accept,
            "Socket activation event received"
        );

        if event.accept {
            // Accept=yes: Spawn a new instance from the template for each connection
            match spawn_service
                .spawn_from_template(&event.service_name, vec![event.fd], None)
                .await
            {
                Ok(instance) => {
                    info!(
                        template = %event.service_name,
                        instance = %instance.name,
                        instance_id = %instance.id,
                        pid = instance.pid,
                        socket = %event.socket_name,
                        fd = event.fd,
                        "New process instance spawned for Accept=yes socket activation"
                    );
                }
                Err(e) => {
                    error!(
                        service = %event.service_name,
                        socket = %event.socket_name,
                        error = %e,
                        "Failed to spawn process instance for Accept=yes socket activation"
                    );
                }
            }
        } else {
            // Accept=no: Start the existing process by name (only if not already running)
            // First check if the process is already running
            let status_query =
                pm_engine::domain::GetProcessStatusQuery::from_name(event.service_name.clone());

            match registry.get_process_status().execute(status_query).await {
                Ok(response) => {
                    if response.process.is_running() {
                        // Process is already running, no need to start it again
                        debug!(
                            service = %event.service_name,
                            pid = response.process.pid(),
                            socket = %event.socket_name,
                            "Process already running, skipping socket activation start"
                        );
                        continue;
                    }
                }
                Err(_) => {
                    // Process not found or error, try to start it anyway
                }
            }

            // Process is not running, start it with FD passing
            let start_cmd =
                StartProcessCommand::from_name_with_fds(event.service_name.clone(), vec![event.fd]);

            match start_use_case.execute(start_cmd).await {
                Ok(response) => {
                    info!(
                        service = %event.service_name,
                        pid = response.pid,
                        socket = %event.socket_name,
                        fd = event.fd,
                        "Process started via socket activation with FD passing"
                    );
                }
                Err(e) => {
                    error!(
                        service = %event.service_name,
                        socket = %event.socket_name,
                        error = %e,
                        "Failed to start process via socket activation"
                    );
                }
            }
        }
    }

    info!("Socket activation event handler stopped");
}

/// Load sockets from configuration directory
///
/// Loads .socket.yaml files from the directory (systemd-style naming convention).
async fn load_sockets_from_config(config_path: &str, socket_manager: Arc<SocketActivationService>) {
    use pm_engine::infrastructure::config::load_sockets_from_path;
    use std::path::PathBuf;

    info!(config_path = %config_path, "Loading sockets from configuration");

    // Load sockets using the unified loader (handles both files and directories)
    let sockets = match load_sockets_from_path(config_path) {
        Ok(s) => s,
        Err(e) => {
            error!(error = %e, "Failed to load sockets from config");
            return;
        }
    };

    if sockets.is_empty() {
        info!("No sockets defined in configuration");
        return;
    }

    info!(count = sockets.len(), "Found socket definitions");

    // Create each socket
    for (socket_name, socket_cfg) in sockets {
        info!(socket = %socket_name, service = %socket_cfg.service, "Creating socket");

        // Build domain socket config
        let mut domain_cfg =
            DomainSocketConfig::new(socket_name.clone(), socket_cfg.service.clone());

        if let Some(ref addr) = socket_cfg.listen_stream {
            domain_cfg = domain_cfg.with_tcp(addr.clone());
        }
        if let Some(ref addr) = socket_cfg.listen_datagram {
            domain_cfg = domain_cfg.with_udp(addr.clone());
        }
        if let Some(ref path) = socket_cfg.listen_unix {
            domain_cfg = domain_cfg.with_unix(PathBuf::from(path));
        }

        domain_cfg = domain_cfg.with_accept(socket_cfg.accept);

        if let Some(ref mode_str) = socket_cfg.socket_mode {
            // Parse octal string (e.g., "660" -> 0o660)
            if let Ok(mode) = u32::from_str_radix(mode_str, 8) {
                domain_cfg = domain_cfg.with_socket_mode(mode);
            }
        }

        if let Some(ref user) = socket_cfg.socket_user {
            domain_cfg = domain_cfg.with_socket_user(user.clone());
        }

        if let Some(ref group) = socket_cfg.socket_group {
            domain_cfg = domain_cfg.with_socket_group(group.clone());
        }

        // Create the socket
        match socket_manager.create_socket(domain_cfg).await {
            Ok(name) => {
                info!(socket = %name, "Socket created successfully");
            }
            Err(e) => {
                error!(socket = %socket_name, error = %e, "Failed to create socket");
            }
        }
    }

    info!("Socket loading complete");
}
