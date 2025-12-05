//! Demonstration of gRPC and REST adapters
//! Run with: cargo run --example adapters_demo

use pm_engine::adapters::{grpc::ProcessManagerService, rest::build_router};
use pm_engine::application::Application;
use pm_engine::infrastructure::{InMemoryProcessRepository, TokioProcessExecutor};
use pm_engine::proto::process_manager::process_manager_server::ProcessManagerServer;
use std::sync::Arc;
use tonic::transport::Server as TonicServer;
use tracing::{info, Level};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt().with_max_level(Level::INFO).init();

    println!("\nProcess Manager - Adapters Demo");
    println!("==========================================\n");

    // 1. Setup Infrastructure Layer
    info!("Setting up infrastructure layer");
    let repository = Arc::new(InMemoryProcessRepository::new());
    let executor = Arc::new(TokioProcessExecutor::new());
    println!("[OK] Infrastructure: InMemoryRepository & TokioExecutor");

    // 2. Setup Application Layer
    info!("Setting up application layer");
    let registry = Arc::new(Application::new(repository, executor));
    println!("[OK] Application: Application with 8 use cases\n");

    // 3. Setup gRPC Adapter (Port 50051)
    let grpc_addr = "0.0.0.0:50051".parse()?;
    let grpc_service = ProcessManagerService::new(registry.clone());
    let grpc_server = TonicServer::builder()
        .add_service(ProcessManagerServer::new(grpc_service))
        .serve(grpc_addr);

    info!("gRPC server configured on {}", grpc_addr);
    println!("[OK] gRPC Adapter: Listening on {}", grpc_addr);

    // 4. Setup REST Adapter (Port 3000)
    let rest_addr = "0.0.0.0:3000";
    let rest_app = build_router(registry.clone());
    let rest_server = axum::Server::bind(&rest_addr.parse()?).serve(rest_app.into_make_service());

    info!("REST server configured on {}", rest_addr);
    println!("[OK] REST Adapter: Listening on http://{}", rest_addr);

    println!("\n==========================================");
    println!("Architecture Overview");
    println!("==========================================\n");
    println!("┌─────────────────────────────────────────┐");
    println!("│       DRIVING ADAPTERS (Input)          │");
    println!("│  gRPC (:50051) │ REST (:3000)           │");
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

    println!("REST API Examples:");
    println!("  curl -X POST http://localhost:3000/processes \\");
    println!("    -H 'Content-Type: application/json' \\");
    println!("    -d '{{\"name\":\"demo\",\"command\":\"/bin/sleep 60\"}}'");
    println!();
    println!("  curl http://localhost:3000/processes");
    println!();
    println!("  curl http://localhost:3000/processes/demo");
    println!();
    println!("  curl -X POST http://localhost:3000/processes/demo/start");
    println!();
    println!("  curl -X POST http://localhost:3000/processes/demo/stop");
    println!();
    println!("  curl -X DELETE http://localhost:3000/processes/demo");
    println!();

    println!("gRPC Examples:");
    println!("  grpcurl -plaintext -d '{{\"name\":\"demo\",\"command\":\"/bin/sleep 60\"}}' \\");
    println!("    [::1]:50051 process_manager.ProcessManager/Create");
    println!();
    println!("  grpcurl -plaintext [::1]:50051 \\");
    println!("    process_manager.ProcessManager/List");
    println!("\n==========================================\n");

    // 5. Run both servers concurrently
    println!("Servers are now running...");
    println!("   Press Ctrl+C to stop\n");

    tokio::select! {
        result = grpc_server => {
            if let Err(e) = result {
                eprintln!("gRPC server error: {}", e);
            }
        }
        result = rest_server => {
            if let Err(e) = result {
                eprintln!("REST server error: {}", e);
            }
        }
    }

    Ok(())
}
