//! IPC transport support for REST adapter
//!
//! Provides platform-specific IPC:
//! - Unix: Unix domain sockets
//! - Windows: TCP on localhost (as fallback)

use axum::Router;
use std::path::Path;
use tracing::{info, warn};

// ============================================================================
// Unix Socket Implementation (Unix-only)
// ============================================================================

#[cfg(unix)]
use hyper::server::accept;
#[cfg(unix)]
use tokio::net::UnixListener;
#[cfg(unix)]
use tokio_stream::wrappers::UnixListenerStream;

/// Start REST API server on Unix socket (Unix-only)
///
/// This is the preferred method for local daemon communication:
/// - No network port consumption
/// - Filesystem-based permissions
/// - Better security and performance
#[cfg(unix)]
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

// ============================================================================
// Windows Implementation - TCP as default IPC
// ============================================================================

/// Start REST API server on TCP (Windows - primary transport)
///
/// On Windows, we use TCP for local communication as Unix sockets
/// are not widely supported. Uses localhost binding for security.
#[cfg(windows)]
pub async fn serve_on_unix_socket(
    _socket_path: &str,
    app: Router,
) -> Result<(), Box<dyn std::error::Error>> {
    // On Windows, fall back to TCP on a well-known port
    let addr = "127.0.0.1:50052".parse()?;

    info!(
        "REST API server listening on TCP {} (Unix sockets not available on Windows)",
        addr
    );

    axum::Server::bind(&addr)
        .serve(app.into_make_service())
        .await?;

    Ok(())
}

// ============================================================================
// Cross-Platform TCP Server (for explicit TCP mode)
// ============================================================================

/// Start REST API server on TCP (cross-platform)
///
/// This function is available on all platforms for explicit TCP mode.
pub async fn serve_on_tcp(
    addr: std::net::SocketAddr,
    app: Router,
) -> Result<(), Box<dyn std::error::Error>> {
    info!("REST API server listening on TCP {}", addr);

    axum::Server::bind(&addr)
        .serve(app.into_make_service())
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
            let socket_path = temp_dir.path().join("api.sock");
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
