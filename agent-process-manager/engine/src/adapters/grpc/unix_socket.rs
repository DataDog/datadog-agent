//! Unix socket support for gRPC adapter

use crate::adapters::grpc::ProcessManagerService;
use crate::proto::process_manager::process_manager_server::ProcessManagerServer;
use std::path::Path;
use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::transport::Server;
use tracing::{info, warn};

/// Start gRPC server on Unix socket
///
/// This is the preferred method for local daemon communication:
/// - No network port consumption
/// - Filesystem-based permissions
/// - Better security and performance
///
/// # Example
///
/// ```no_run
/// use pm_engine::adapters::grpc::{ProcessManagerService, unix_socket::serve_on_unix_socket};
///
/// # async fn example() {
/// # let service = ProcessManagerService::new(todo!());
/// serve_on_unix_socket("/var/run/process-manager/pm.sock", service)
///     .await
///     .unwrap();
/// # }
/// ```
pub async fn serve_on_unix_socket(
    socket_path: &str,
    service: ProcessManagerService,
) -> Result<(), Box<dyn std::error::Error>> {
    let path = Path::new(socket_path);

    // Remove socket file if it already exists
    if path.exists() {
        info!("Removing existing socket file: {}", socket_path);
        std::fs::remove_file(path)?;
    }

    // Create parent directory if needed
    if let Some(parent) = path.parent() {
        if !parent.exists() {
            info!("Creating socket directory: {}", parent.display());
            std::fs::create_dir_all(parent)?;
        }
    }

    // Bind Unix listener
    let listener = UnixListener::bind(socket_path)?;

    // Set appropriate permissions (0660 - owner and group can read/write)
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let permissions = std::fs::Permissions::from_mode(0o660);
        std::fs::set_permissions(socket_path, permissions)?;
    }

    info!("gRPC server listening on Unix socket: {}", socket_path);

    // Build reflection service for grpcurl support
    let reflection_service = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(crate::proto::process_manager::FILE_DESCRIPTOR_SET)
        .build()
        .unwrap();

    // Serve on Unix socket with reflection
    Server::builder()
        .add_service(ProcessManagerServer::new(service))
        .add_service(reflection_service)
        .serve_with_incoming(UnixListenerStream::new(listener))
        .await?;

    // Cleanup socket on exit
    if path.exists() {
        warn!("Cleaning up socket file: {}", socket_path);
        let _ = std::fs::remove_file(path);
    }

    Ok(())
}

/// Start gRPC server on Unix socket with health checking
///
/// This variant includes the standard gRPC health checking service
pub async fn serve_on_unix_socket_with_health<H>(
    socket_path: &str,
    service: ProcessManagerService,
    health_service: H,
) -> Result<(), Box<dyn std::error::Error>>
where
    H: tonic::codegen::Service<
            http::Request<hyper::Body>,
            Response = http::Response<tonic::body::BoxBody>,
            Error = std::convert::Infallible,
        > + tonic::server::NamedService
        + Clone
        + Send
        + 'static,
    H::Future: Send + 'static,
{
    let path = Path::new(socket_path);

    // Remove socket file if it already exists
    if path.exists() {
        info!("Removing existing socket file: {}", socket_path);
        std::fs::remove_file(path)?;
    }

    // Create parent directory if needed
    if let Some(parent) = path.parent() {
        if !parent.exists() {
            info!("Creating socket directory: {}", parent.display());
            std::fs::create_dir_all(parent)?;
        }
    }

    // Bind Unix listener
    let listener = UnixListener::bind(socket_path)?;

    // Set appropriate permissions (0660 - owner and group can read/write)
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let permissions = std::fs::Permissions::from_mode(0o660);
        std::fs::set_permissions(socket_path, permissions)?;
    }

    info!("gRPC server listening on Unix socket: {}", socket_path);

    // Build reflection service for grpcurl support
    let reflection_service = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(crate::proto::process_manager::FILE_DESCRIPTOR_SET)
        .build()
        .unwrap();

    // Serve on Unix socket with health and reflection
    Server::builder()
        .add_service(health_service)
        .add_service(ProcessManagerServer::new(service))
        .add_service(reflection_service)
        .serve_with_incoming(UnixListenerStream::new(listener))
        .await?;

    // Cleanup socket on exit
    if path.exists() {
        warn!("Cleaning up socket file: {}", socket_path);
        let _ = std::fs::remove_file(path);
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[tokio::test]
    async fn test_socket_path_validation() {
        let temp_dir = TempDir::new().unwrap();
        let socket_path = temp_dir.path().join("test.sock");
        let path_str = socket_path.to_str().unwrap();

        // Path should not exist initially
        assert!(!Path::new(path_str).exists());
    }
}
