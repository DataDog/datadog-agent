//! Unix socket support for REST adapter

use axum::Router;
use hyper::server::accept;
use std::path::Path;
use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;
use tracing::{info, warn};

/// Start REST API server on Unix socket
///
/// This is the preferred method for local daemon communication:
/// - No network port consumption
/// - Filesystem-based permissions
/// - Better security and performance
///
/// # Example
///
/// ```no_run
/// use pm_engine::adapters::rest::{build_router, unix_socket::serve_on_unix_socket};
///
/// # async fn example() {
/// # let router = build_router(todo!());
/// serve_on_unix_socket("/var/run/process-manager/api.sock", router)
///     .await
///     .unwrap();
/// # }
/// ```
pub async fn serve_on_unix_socket(
    socket_path: &str,
    app: Router,
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

    info!("REST API server listening on Unix socket: {}", socket_path);

    // Serve on Unix socket using hyper
    let stream = UnixListenerStream::new(listener);
    axum::Server::builder(accept::from_stream(stream))
        .serve(app.into_make_service())
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
        let socket_path = temp_dir.path().join("api.sock");
        let path_str = socket_path.to_str().unwrap();

        // Path should not exist initially
        assert!(!Path::new(path_str).exists());
    }
}
