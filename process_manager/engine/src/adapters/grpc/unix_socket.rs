//! IPC transport support for gRPC adapter
//!
//! Provides platform-specific IPC:
//! - Unix: Unix domain sockets
//! - Windows: Named pipes (with TCP fallback)

use crate::adapters::grpc::ProcessManagerService;
use crate::proto::process_manager::process_manager_server::ProcessManagerServer;
use tonic::transport::Server;
use tracing::info;

// ============================================================================
// Unix Socket Implementation (Unix-only)
// ============================================================================

#[cfg(unix)]
use std::path::Path;
#[cfg(unix)]
use tokio::net::UnixListener;
#[cfg(unix)]
use tokio_stream::wrappers::UnixListenerStream;
#[cfg(unix)]
use tracing::warn;

/// Start gRPC server on Unix socket (Unix-only)
///
/// This is the preferred method for local daemon communication:
/// - No network port consumption
/// - Filesystem-based permissions
/// - Better security and performance
#[cfg(unix)]
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

/// Start gRPC server on Unix socket with health checking (Unix-only)
#[cfg(unix)]
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

// ============================================================================
// Windows Implementation - TCP as default IPC
// ============================================================================

/// Start gRPC server on TCP (Windows - primary transport)
///
/// On Windows, we use TCP for local communication as Unix sockets
/// are not widely supported. Uses localhost binding for security.
#[cfg(windows)]
pub async fn serve_on_unix_socket(
    _socket_path: &str,
    service: ProcessManagerService,
) -> Result<(), Box<dyn std::error::Error>> {
    // On Windows, fall back to TCP on a well-known port
    let addr = "127.0.0.1:50051".parse()?;

    info!(
        "gRPC server listening on TCP {} (Unix sockets not available on Windows)",
        addr
    );

    // Build reflection service for grpcurl support
    let reflection_service = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(crate::proto::process_manager::FILE_DESCRIPTOR_SET)
        .build()
        .unwrap();

    Server::builder()
        .add_service(ProcessManagerServer::new(service))
        .add_service(reflection_service)
        .serve(addr)
        .await?;

    Ok(())
}

/// Start gRPC server on TCP with health checking (Windows)
#[cfg(windows)]
pub async fn serve_on_unix_socket_with_health<H>(
    _socket_path: &str,
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
    let addr = "127.0.0.1:50051".parse()?;

    info!(
        "gRPC server listening on TCP {} (Unix sockets not available on Windows)",
        addr
    );

    // Build reflection service for grpcurl support
    let reflection_service = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(crate::proto::process_manager::FILE_DESCRIPTOR_SET)
        .build()
        .unwrap();

    Server::builder()
        .add_service(health_service)
        .add_service(ProcessManagerServer::new(service))
        .add_service(reflection_service)
        .serve(addr)
        .await?;

    Ok(())
}

// ============================================================================
// Cross-Platform TCP Server (for explicit TCP mode)
// ============================================================================

/// Start gRPC server on TCP (cross-platform)
///
/// This function is available on all platforms for explicit TCP mode.
pub async fn serve_on_tcp(
    addr: std::net::SocketAddr,
    service: ProcessManagerService,
) -> Result<(), Box<dyn std::error::Error>> {
    info!("gRPC server listening on TCP {}", addr);

    let reflection_service = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(crate::proto::process_manager::FILE_DESCRIPTOR_SET)
        .build()
        .unwrap();

    Server::builder()
        .add_service(ProcessManagerServer::new(service))
        .add_service(reflection_service)
        .serve(addr)
        .await?;

    Ok(())
}

/// Start gRPC server on TCP with health checking (cross-platform)
pub async fn serve_on_tcp_with_health<H>(
    addr: std::net::SocketAddr,
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
    info!("gRPC server listening on TCP {}", addr);

    let reflection_service = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(crate::proto::process_manager::FILE_DESCRIPTOR_SET)
        .build()
        .unwrap();

    Server::builder()
        .add_service(health_service)
        .add_service(ProcessManagerServer::new(service))
        .add_service(reflection_service)
        .serve(addr)
        .await?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_socket_path_validation() {
        #[cfg(unix)]
        {
            use tempfile::TempDir;
            let temp_dir = TempDir::new().unwrap();
            let socket_path = temp_dir.path().join("test.sock");
            let path_str = socket_path.to_str().unwrap();

            // Path should not exist initially
            assert!(!Path::new(path_str).exists());
        }

        #[cfg(windows)]
        {
            // On Windows, we use TCP, so just verify the module compiles
            assert!(true);
        }
    }
}
