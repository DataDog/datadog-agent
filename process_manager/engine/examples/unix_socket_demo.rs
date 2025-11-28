//! Demonstration of Unix socket adapters (daemon mode)
//! Run with: cargo run --example unix_socket_demo

use pm_engine::adapters::grpc::{
    unix_socket::serve_on_unix_socket as grpc_unix, ProcessManagerService,
};
use pm_engine::adapters::rest::{build_router, unix_socket::serve_on_unix_socket as rest_unix};
use pm_engine::application::Application;
use pm_engine::infrastructure::{InMemoryProcessRepository, TokioProcessExecutor};
use std::sync::Arc;
use tracing::{info, Level};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt().with_max_level(Level::INFO).init();

    println!("\nProcess Manager - Unix Socket Demo (Daemon Mode)");
    println!("============================================================\n");

    // 1. Setup Infrastructure Layer
    info!("Setting up infrastructure layer");
    let repository = Arc::new(InMemoryProcessRepository::new());
    let executor = Arc::new(TokioProcessExecutor::new());
    println!("[OK] Infrastructure: InMemoryRepository & TokioExecutor");

    // 2. Setup Application Layer
    info!("Setting up application layer");
    let registry = Arc::new(Application::new(repository, executor));
    println!("[OK] Application: Application with 8 use cases\n");

    // 3. Define Unix socket paths (systemd-style)
    let grpc_socket = "/tmp/process-manager.sock"; // Or /var/run/process-manager/pm.sock in production
    let rest_socket = "/tmp/process-manager-api.sock"; // Or /var/run/process-manager/api.sock

    println!("Using Unix Sockets (daemon mode):");
    println!("   gRPC: {}", grpc_socket);
    println!("   REST: {}\n", rest_socket);

    // 4. Setup gRPC on Unix socket
    let grpc_service = ProcessManagerService::new(registry.clone());
    let grpc_server = async move {
        if let Err(e) = grpc_unix(grpc_socket, grpc_service).await {
            eprintln!("gRPC server error: {}", e);
        }
    };

    println!("[OK] gRPC Adapter: Listening on {}", grpc_socket);

    // 5. Setup REST on Unix socket
    let rest_app = build_router(registry.clone());
    let rest_server = async move {
        if let Err(e) = rest_unix(rest_socket, rest_app).await {
            eprintln!("REST server error: {}", e);
        }
    };

    println!("[OK] REST Adapter: Listening on {}", rest_socket);

    println!("\n==========================================");
    println!("Architecture Overview");
    println!("==========================================\n");
    println!("┌─────────────────────────────────────────┐");
    println!("│       DRIVING ADAPTERS (Input)          │");
    println!("│  gRPC (Unix) │ REST (Unix)             │");
    println!("└────────────────┬────────────────────────┘");
    println!("                 │");
    println!("┌────────────────▼────────────────────────┐");
    println!("│         APPLICATION LAYER                │");
    println!("│      Application (8 use cases)       │");
    println!("└────────────────┬────────────────────────┘");
    println!("                 │");
    println!("┌────────────────▼────────────────────────┐");
    println!("│          DOMAIN LAYER                    │");
    println!("│  Entities │ Values │ Ports │ Logic       │");
    println!("└────────────────┬────────────────────────┘");
    println!("                 │");
    println!("┌────────────────▼────────────────────────┐");
    println!("│    INFRASTRUCTURE (Driven Adapters)      │");
    println!("│  TokioExecutor │ InMemoryRepository      │");
    println!("└─────────────────────────────────────────┘\n");

    println!("==========================================");
    println!("Try these commands:");
    println!("==========================================\n");

    println!("REST API Examples (using Unix socket):");
    println!("  curl --unix-socket {} \\", rest_socket);
    println!("    -X POST http://localhost/processes \\");
    println!("    -H 'Content-Type: application/json' \\");
    println!("    -d '{{\"name\":\"demo\",\"command\":\"/bin/sleep 60\"}}'");
    println!();
    println!("  curl --unix-socket {} \\", rest_socket);
    println!("    http://localhost/processes");
    println!();
    println!("  curl --unix-socket {} \\", rest_socket);
    println!("    http://localhost/processes/demo");
    println!();
    println!("  curl --unix-socket {} \\", rest_socket);
    println!("    -X POST http://localhost/processes/demo/start");
    println!();

    println!("gRPC Examples (using Unix socket):");
    println!("  grpcurl -unix -plaintext \\");
    println!("    -d '{{\"name\":\"demo\",\"command\":\"/bin/sleep 60\"}}' \\");
    println!("    {} process_manager.ProcessManager/Create", grpc_socket);
    println!();
    println!("  grpcurl -unix -plaintext \\");
    println!("    {} process_manager.ProcessManager/List", grpc_socket);
    println!();

    println!("Benefits of Unix Sockets:");
    println!("  - No TCP port consumption");
    println!("  - Filesystem-based permissions (0660)");
    println!("  - Better security (no network exposure)");
    println!("  - Better performance for local IPC");
    println!("  - Standard for systemd-style daemons");
    println!("\n==========================================\n");

    // 6. Run both servers concurrently
    println!("Servers are now running...");
    println!("   Press Ctrl+C to stop\n");

    tokio::select! {
        _ = grpc_server => {
            info!("gRPC server terminated");
        }
        _ = rest_server => {
            info!("REST server terminated");
        }
    }

    Ok(())
}
