use pm_engine::proto::process_manager_server::{ProcessManager, ProcessManagerServer};

struct ProcessManagerSvc;

#[tonic::async_trait]
impl ProcessManager for ProcessManagerSvc {
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let service = ProcessManagerServer::new(ProcessManagerSvc);
    pm_engine::transport::serve_default(service, shutdown_signal()).await?;

    Ok(())
}

async fn shutdown_signal() {
    let _ = tokio::signal::ctrl_c().await;
}
