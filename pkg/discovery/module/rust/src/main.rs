// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Correctness
#![deny(clippy::indexing_slicing)]
#![deny(clippy::string_slice)]
#![deny(clippy::cast_possible_wrap)]
#![deny(clippy::undocumented_unsafe_blocks)]
// Panicking code
#![deny(clippy::unwrap_used)]
#![deny(clippy::expect_used)]
#![deny(clippy::panic)]
#![deny(clippy::unimplemented)]
#![deny(clippy::todo)]
// Debug code that shouldn't be in production
#![deny(clippy::dbg_macro)]
#![deny(clippy::print_stdout)]
#![deny(clippy::print_stderr)]

use std::env;
use std::fs::Permissions;
use std::io::ErrorKind;
use std::os::unix::fs::{PermissionsExt, chown};
use std::path::Path;

use anyhow::{Context, Result, anyhow};
use dd_discovery::{Params, get_services};

use http_body_util::combinators::BoxBody;
use http_body_util::{BodyExt, Full};
use hyper::body::Bytes;
use hyper::header::CONTENT_TYPE;
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Method, Request, Response, StatusCode};
use hyper_util::rt::TokioIo;
use log::{debug, error, info, warn};
use serde_json::json;
use tokio::net::UnixListener;
use tokio::signal::unix::{SignalKind, signal};

mod cli;

use cli::Args;

static BADREQUEST: &[u8] = b"Bad request";
static NOTFOUND: &[u8] = b"Not found";

fn remove_pid_file(path: &Path) {
    if let Err(e) = std::fs::remove_file(path) {
        error!("Failed to remove PID file: {}", e);
    } else {
        info!("Removed PID file at {}", path.display());
    }
}

fn setup_socket(socket_path: &str) -> Result<UnixListener> {
    std::fs::remove_file(socket_path)
        .or_else(|error| {
            if error.kind() == ErrorKind::NotFound {
                Ok(())
            } else {
                Err(error)
            }
        })
        .context("failed to remove existing socket")?;

    let sock = UnixListener::bind(socket_path).context("could not create socket")?;
    std::fs::set_permissions(socket_path, Permissions::from_mode(0o720))
        .context("could not set socket permissions")?;

    // Try to chown to dd-agent user if it exists, skip if it doesn't
    if let Some(agent_user) = uzers::get_user_by_name("dd-agent") {
        if let Err(e) = chown(
            socket_path,
            Some(agent_user.uid()),
            Some(agent_user.primary_group_id()),
        ) {
            warn!("could not set socket ownership: {e}")
        }
    } else {
        info!("dd-agent user not found, skipping socket ownership change");
    }

    Ok(sock)
}

async fn handle_services(
    req: Request<hyper::body::Incoming>,
) -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    if req
        .headers()
        .get(CONTENT_TYPE)
        .is_none_or(|value| value != "application/json")
    {
        return bad_request();
    }

    let body = match req.collect().await {
        Ok(body) => body.to_bytes(),
        Err(e) => {
            error!("Failed to read request body: {e}");
            return bad_request();
        }
    };

    let params: Params = match serde_json::from_slice(&body) {
        Ok(params) => params,
        Err(e) => {
            error!("Failed to parse JSON params: {e}");
            return bad_request();
        }
    };

    let services = get_services(params);
    debug!("Found {} services", services.services.len());

    Response::builder()
        .header("Content-Type", "application/json")
        .body(
            Full::new(
                serde_json::to_vec(&services)
                    .unwrap_or_else(|e| {
                        error!("Failed to serialize response: {e}");
                        b"Internal server error".to_vec()
                    })
                    .into(),
            )
            .map_err(|e| match e {})
            .boxed(),
        )
        .map_err(|e| anyhow!("Failed to build response: {}", e))
}

async fn handle_debug_stats() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    Response::builder()
        .header("Content-Type", "application/json")
        .body(
            Full::new(
                serde_json::to_vec(&json!({}))
                    .unwrap_or_else(|e| {
                        error!("Failed to serialize response: {e}");
                        b"Internal server error".to_vec()
                    })
                    .into(),
            )
            .map_err(|e| match e {})
            .boxed(),
        )
        .map_err(|e| anyhow!("Failed to build response: {}", e))
}

fn bad_request() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    Response::builder()
        .status(StatusCode::BAD_REQUEST)
        .body(Full::new(BADREQUEST.into()).map_err(|e| match e {}).boxed())
        .map_err(|e| anyhow!("Failed to build bad request response: {}", e))
}

fn not_found() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    Response::builder()
        .status(StatusCode::NOT_FOUND)
        .body(Full::new(NOTFOUND.into()).map_err(|e| match e {}).boxed())
        .map_err(|e| anyhow!("Failed to build not found response: {}", e))
}

async fn handle_request(
    req: Request<hyper::body::Incoming>,
) -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    match (req.method(), req.uri().path()) {
        (&Method::POST, "/discovery/services") => {
            debug!("Handling /discovery/services request");
            handle_services(req).await
        }
        (&Method::GET, "/debug/stats") => handle_debug_stats().await,
        _ => {
            info!(
                "{} Request to unknown endpoint: {}",
                req.method(),
                req.uri().path()
            );
            not_found()
        }
    }
}

async fn run_system_probe_lite(socket_path: &str) -> Result<()> {
    info!("Using sysprobe socket path: {}", socket_path);
    let sock = setup_socket(socket_path).context("Failed to setup Unix socket")?;

    // Setup signal handlers
    let mut sigterm = signal(SignalKind::terminate()).context("Failed to setup SIGTERM handler")?;
    let mut sigint = signal(SignalKind::interrupt()).context("Failed to setup SIGINT handler")?;

    loop {
        tokio::select! {
            // Handle incoming connections
            accept_result = sock.accept() => {
                let (stream, _) = accept_result?;

                // Use an adapter to access something implementing `tokio::io` traits as if they
                // implement `hyper::rt` IO traits.
                let io = TokioIo::new(stream);

                // Spawn a tokio task to serve multiple connections concurrently
                tokio::task::spawn(async move {
                    if let Err(err) = http1::Builder::new()
                        // `service_fn` converts our function in a `Service`
                        .serve_connection(
                            io,
                            service_fn(|req| async {
                                Ok::<_, anyhow::Error>(handle_request(req).await.unwrap_or_else(|e| {
                                    error!("Request handling failed: {e}");
                                    // Return an internal server error response
                                    Response::builder()
                                        .status(StatusCode::INTERNAL_SERVER_ERROR)
                                        .body(
                                            Full::new(Bytes::from(&b"Internal Server Error"[..]))
                                                .map_err(|e| match e {})
                                                .boxed(),
                                        )
                                        .unwrap_or_else(|_| {
                                            // Last resort if even error response building fails
                                            Response::new(
                                                Full::new(Bytes::from(&b"Error"[..]))
                                                    .map_err(|e| match e {})
                                                    .boxed(),
                                            )
                                        })
                                }))
                            }),
                        )
                        .await
                    {
                        error!("Error serving connection: {err}");
                    }
                });
            }
            // Handle SIGTERM
            _ = sigterm.recv() => {
                info!("Received SIGTERM, shutting down");
                return Ok(());
            }
            // Handle SIGINT
            _ = sigint.recv() => {
                info!("Received SIGINT, shutting down");
                return Ok(());
            }
        }
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse(env::args())?;
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "SYS-PROBE-LITE",
        level: args.log_level,
        log_file: args.log_file.clone(),
    })?;
    info!("Starting system-probe-lite");

    let result = run_system_probe_lite(&args.socket_path).await;

    // Cleanup PID file on exit (defer pattern)
    // This ensures cleanup happens regardless of how we exit (signal, error, or normal completion)
    if let Some(path) = args.pid_path {
        remove_pid_file(&path);
    }

    result
}

#[cfg(test)]
#[allow(clippy::panic)] // Tests are allowed to use panic for test failures
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    #[test]
    fn test_remove_pid_file_deletes_file() {
        let temp_dir =
            TempDir::new().unwrap_or_else(|e| panic!("Failed to create temp dir: {}", e));
        let pid_path = temp_dir.path().join("test.pid");

        // Create a PID file
        fs::write(&pid_path, "12345")
            .unwrap_or_else(|e| panic!("Failed to create test file: {}", e));
        assert!(pid_path.exists(), "Test file should exist before removal");

        // Remove it
        remove_pid_file(&pid_path);

        assert!(!pid_path.exists(), "PID file should be deleted");
    }

    #[test]
    fn test_remove_pid_file_handles_nonexistent() {
        let temp_dir =
            TempDir::new().unwrap_or_else(|e| panic!("Failed to create temp dir: {}", e));
        let nonexistent_path = temp_dir.path().join("nonexistent.pid");

        // Should not panic
        remove_pid_file(&nonexistent_path);

        // Should still not exist
        assert!(
            !nonexistent_path.exists(),
            "Nonexistent file should remain nonexistent"
        );
    }
}
