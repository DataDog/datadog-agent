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
use std::fs::{File, Permissions};
use std::io::{ErrorKind, IoSlice};
use std::os::fd::AsRawFd;
use std::os::unix::fs::{PermissionsExt, chown};
use std::path::Path;
use std::sync::atomic::{AtomicBool, Ordering};

use anyhow::{Context, Result, anyhow};
use dd_discovery::{Params, get_services, privileged_logs};

use http_body_util::combinators::BoxBody;
use http_body_util::{BodyExt, Full};
use hyper::body::Bytes;
use hyper::header::{CONNECTION, CONTENT_TYPE, UPGRADE};
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::upgrade::OnUpgrade;
use hyper::{Method, Request, Response, StatusCode};
use hyper_util::rt::TokioIo;
use log::{debug, error, info, warn};
use nix::sys::socket::{ControlMessage, MsgFlags, sendmsg};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tokio::io::Interest;
use tokio::net::{UnixListener, UnixStream};
use tokio::signal::unix::{SignalKind, signal};
use tokio::sync::Semaphore;

mod cli;

use cli::Args;

/// We choose 2 because one is for regular agent checks and another one is for manual troubleshooting.
/// This matches the Go system-probe's DefaultMaxConcurrentRequests.
static SERVICES_SEMAPHORE: Semaphore = Semaphore::const_new(2);

/// Set from --privileged-logs: the endpoint only exists when system-probe has
/// the privileged_logs module enabled.
static PRIVILEGED_LOGS: AtomicBool = AtomicBool::new(false);

/// Protocol that the connection switches to for passing a file descriptor.
const UPGRADE_PROTOCOL: &str = "dd-privileged-logs";

/// Body of `POST /privileged_logs/open`.
#[derive(Deserialize)]
struct OpenFileRequest {
    path: String,
    #[serde(default)]
    no_follow: bool,
}

static BADREQUEST: &str = "Bad request";
static NOTFOUND: &str = "Not found";

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

/// Drops all Linux capabilities except CAP_SYS_PTRACE and CAP_DAC_READ_SEARCH
/// from every capability set.
///
/// Both capabilities are required for full service discovery:
/// - CAP_SYS_PTRACE: open /proc/<pid>/root and /proc/<pid>/{fd,maps,exe,environ}
/// - CAP_DAC_READ_SEARCH: traverse restricted directories (e.g. mode-750 home dirs,
///   container app directories) when reading files through /proc/<pid>/root/
///
/// Must be called after socket setup (which needs CAP_CHOWN for chown).
/// Failure is non-fatal: SPL logs a warning and continues with inherited capabilities
/// rather than refusing to start.
///
/// Sets modified:
/// - Effective:    {CAP_SYS_PTRACE, CAP_DAC_READ_SEARCH} (or subset if not in permitted)
/// - Permitted:    {CAP_SYS_PTRACE, CAP_DAC_READ_SEARCH} (or subset)
/// - Inheritable:  {} (empty — exec'd children get nothing)
/// - Ambient:      {} (empty — requires kernel ≥ 4.3, silently skipped otherwise)
/// - Bounding:     {CAP_SYS_PTRACE, CAP_DAC_READ_SEARCH} (prevents future escalation)
fn drop_capabilities() {
    match try_drop_capabilities() {
        Ok(true) => info!("Capabilities restricted to {{CAP_SYS_PTRACE, CAP_DAC_READ_SEARCH}}"),
        Ok(false) => {
            warn!("Not all required capabilities were available; service discovery may be impaired")
        }
        Err(e) => {
            warn!("Failed to restrict capabilities: {e:#}; continuing with inherited capabilities")
        }
    }
}

fn try_drop_capabilities() -> Result<bool> {
    use caps::{CapSet, Capability};

    let current_permitted = caps::read(None, CapSet::Permitted)?;

    // CAP_SYS_PTRACE: needed to follow /proc/<pid>/root and open /proc/<pid>/ files.
    // CAP_DAC_READ_SEARCH: needed to traverse restricted directories inside process
    // root filesystems (e.g. mode-750 home dirs on Ubuntu 22.04+, container app dirs).
    let has_ptrace = current_permitted.contains(&Capability::CAP_SYS_PTRACE);
    let has_dac = current_permitted.contains(&Capability::CAP_DAC_READ_SEARCH);

    let mut keep = caps::CapsHashSet::new();
    if has_ptrace {
        keep.insert(Capability::CAP_SYS_PTRACE);
    } else {
        warn!(
            "CAP_SYS_PTRACE is not in the permitted set; /proc access for other processes will fail"
        );
    }
    if has_dac {
        keep.insert(Capability::CAP_DAC_READ_SEARCH);
    } else {
        warn!(
            "CAP_DAC_READ_SEARCH is not in the permitted set; reading files in restricted directories may fail"
        );
    }

    // Clear inheritable set (children cannot inherit capabilities).
    caps::clear(None, CapSet::Inheritable)?;

    // Clear ambient set (kernel ≥ 4.3; older kernels return EINVAL, which we ignore).
    let _ = caps::clear(None, CapSet::Ambient);

    // Lower the bounding set BEFORE restricting effective/permitted, because
    // CAP_SETPCAP (needed for bounding set drops) must still be in the effective
    // set at this point. Ignores errors for capabilities absent from the bounding set.
    for cap in caps::all() {
        if !keep.contains(&cap) {
            let _ = caps::drop(None, CapSet::Bounding, cap);
        }
    }

    // Restrict effective and permitted to {CAP_SYS_PTRACE, CAP_DAC_READ_SEARCH} (or subset).
    caps::set(None, CapSet::Effective, &keep)?;
    caps::set(None, CapSet::Permitted, &keep)?;

    Ok(has_ptrace && has_dac)
}

async fn handle_services<B>(req: Request<B>) -> Result<Response<BoxBody<Bytes, std::io::Error>>>
where
    B: hyper::body::Body<Data = Bytes>,
    B::Error: std::fmt::Display,
{
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

    let services = tokio::task::spawn_blocking(|| get_services(params)).await?;
    debug!("Found {} services", services.services.len());

    json_response(&services)
}

async fn handle_state() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    json_response(&json!({
        "implementation": "system-probe-lite",
    }))
}

async fn handle_config() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    // SPL only runs when discovery.enabled and discovery.use_system_probe_lite are both true,
    // so we can hardcode these values.
    let yaml_config = "discovery:\n  enabled: true\n  use_system_probe_lite: true\n";
    text_response(StatusCode::OK, yaml_config)
}

async fn handle_config_by_source() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    json_response(&json!({
        "default": {
            "discovery": {
                "enabled": true,
                "use_system_probe_lite": true
            }
        }
    }))
}

async fn handle_debug_stats() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    json_response(&json!({}))
}

/// Opens the requested log file and, once the connection has switched
/// protocols, passes its file descriptor to the client.
async fn handle_privileged_logs_open<B>(
    mut req: Request<B>,
) -> Result<Response<BoxBody<Bytes, std::io::Error>>>
where
    B: hyper::body::Body<Data = Bytes>,
    B::Error: std::fmt::Display,
{
    // Must be taken before the body is consumed.
    let on_upgrade = hyper::upgrade::on(&mut req);

    let body = match req.collect().await {
        Ok(body) => body.to_bytes(),
        Err(e) => {
            error!("Failed to read request body: {e}");
            return bad_request();
        }
    };

    let open_req: OpenFileRequest = match serde_json::from_slice(&body) {
        Ok(open_req) => open_req,
        Err(e) => {
            error!("Failed to parse request: {e}");
            return bad_request();
        }
    };

    debug!("Received request to open file: {}", open_req.path);
    // Filesystem calls can block, so keep them off the event loop that serves
    // every endpoint.
    let opened = tokio::task::spawn_blocking(move || {
        privileged_logs::open_log_file(&open_req.path, open_req.no_follow)
    });
    let file = match opened.await? {
        Ok(file) => file,
        Err(e) => {
            error!("{e:#}");
            return text_response(StatusCode::INTERNAL_SERVER_ERROR, format!("{e:#}"));
        }
    };

    tokio::task::spawn(async move {
        if let Err(e) = send_fd_after_upgrade(on_upgrade, file).await {
            error!("Failed to send file descriptor: {e:#}");
        }
    });

    Response::builder()
        .status(StatusCode::SWITCHING_PROTOCOLS)
        .header(CONNECTION, "Upgrade")
        .header(UPGRADE, UPGRADE_PROTOCOL)
        .body(full(Bytes::new()))
        .map_err(|e| anyhow!("Failed to build response: {}", e))
}

/// Sends the file descriptor as SCM_RIGHTS over the upgraded connection, which
/// is closed afterwards.
async fn send_fd_after_upgrade(on_upgrade: OnUpgrade, file: File) -> Result<()> {
    let upgraded = on_upgrade.await?;
    // Connections are served from a TokioIo<UnixStream> in run_system_probe_lite.
    let stream = upgraded
        .downcast::<TokioIo<UnixStream>>()
        .map_err(|_| anyhow!("upgraded connection is not a Unix socket"))?
        .io
        .into_inner();
    let payload = [IoSlice::new(br#"{"success":true}"#)];
    let fds = [file.as_raw_fd()];
    let rights = [ControlMessage::ScmRights(&fds)];
    let flags = MsgFlags::MSG_NOSIGNAL;
    stream
        .async_io(Interest::WRITABLE, || {
            sendmsg::<()>(stream.as_raw_fd(), &payload, &rights, flags, None)
                .map_err(std::io::Error::from)
        })
        .await?;
    Ok(())
}

fn full(body: impl Into<Bytes>) -> BoxBody<Bytes, std::io::Error> {
    Full::new(body.into()).map_err(|e| match e {}).boxed()
}

fn json_response(value: &impl Serialize) -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    let body = serde_json::to_vec(value).unwrap_or_else(|e| {
        error!("Failed to serialize response: {e}");
        b"Internal server error".to_vec()
    });
    Response::builder()
        .header(CONTENT_TYPE, "application/json")
        .body(full(body))
        .map_err(|e| anyhow!("Failed to build response: {}", e))
}

fn text_response(
    status: StatusCode,
    body: impl Into<Bytes>,
) -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    Response::builder()
        .status(status)
        .body(full(body))
        .map_err(|e| anyhow!("Failed to build response: {}", e))
}

fn bad_request() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    text_response(StatusCode::BAD_REQUEST, BADREQUEST)
}

fn not_found() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    text_response(StatusCode::NOT_FOUND, NOTFOUND)
}

fn too_many_requests() -> Result<Response<BoxBody<Bytes, std::io::Error>>> {
    text_response(StatusCode::TOO_MANY_REQUESTS, "Too many requests")
}

async fn handle_request<B>(req: Request<B>) -> Result<Response<BoxBody<Bytes, std::io::Error>>>
where
    B: hyper::body::Body<Data = Bytes>,
    B::Error: std::fmt::Display,
{
    match (req.method(), req.uri().path()) {
        (&Method::POST, "/discovery/services") => {
            debug!("Handling /discovery/services request");
            let _permit = match SERVICES_SEMAPHORE.try_acquire() {
                Ok(permit) => permit,
                Err(_) => {
                    warn!("rejecting request for path=/discovery/services concurrency_limit=2");
                    return too_many_requests();
                }
            };
            handle_services(req).await
        }
        (&Method::GET, "/discovery/state") => handle_state().await,
        (&Method::GET, "/config") => handle_config().await,
        (&Method::GET, "/config/by-source") => handle_config_by_source().await,
        (&Method::GET, "/debug/stats") => handle_debug_stats().await,
        (&Method::POST, "/privileged_logs/open") if PRIVILEGED_LOGS.load(Ordering::Relaxed) => {
            handle_privileged_logs_open(req).await
        }
        _ => {
            debug!(
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

    drop_capabilities();

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
                                    error!("Request handling failed: {e:#}");
                                    text_response(StatusCode::INTERNAL_SERVER_ERROR, "Internal Server Error")
                                        // Last resort if even error response building fails
                                        .unwrap_or_else(|_| Response::new(full("Error")))
                                }))
                            }),
                        )
                        .with_upgrades()
                        .await
                    {
                        error!("Error serving connection: {:#}", anyhow::Error::new(err));
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

#[tokio::main(flavor = "current_thread")]
async fn main() -> Result<()> {
    let args = Args::parse(env::args())?;
    dd_agent_log::init(dd_agent_log::LogConfig {
        logger_name: "SYS-PROBE-LITE",
        level: args.log_level,
        log_file: args.log_file.clone(),
    })?;
    info!("Starting system-probe-lite");
    for arg in &args.unknown_args {
        warn!("unknown argument: {arg}");
    }

    PRIVILEGED_LOGS.store(args.privileged_logs, Ordering::Relaxed);
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

    fn services_request() -> Request<Full<Bytes>> {
        Request::builder()
            .method(Method::POST)
            .uri("/discovery/services")
            .header(CONTENT_TYPE, "application/json")
            .body(Full::new(Bytes::from("{}")))
            .unwrap_or_else(|e| panic!("Failed to build request: {e}"))
    }

    #[tokio::test]
    async fn test_concurrency_limit() {
        // Acquire both permits to saturate the semaphore (limit=2).
        let permit1 = SERVICES_SEMAPHORE
            .try_acquire()
            .unwrap_or_else(|e| panic!("Failed to acquire first permit: {e}"));
        let permit2 = SERVICES_SEMAPHORE
            .try_acquire()
            .unwrap_or_else(|e| panic!("Failed to acquire second permit: {e}"));

        // With both permits held, handle_request should return 429.
        let resp = handle_request(services_request())
            .await
            .unwrap_or_else(|e| panic!("handle_request failed: {e}"));
        assert_eq!(
            resp.status(),
            StatusCode::TOO_MANY_REQUESTS,
            "Expected 429 when semaphore is exhausted"
        );

        // Other endpoints should still work even with both permits held.
        for path in [
            "/discovery/state",
            "/config",
            "/config/by-source",
            "/debug/stats",
        ] {
            let req = Request::builder()
                .method(Method::GET)
                .uri(path)
                .body(Full::new(Bytes::new()))
                .unwrap_or_else(|e| panic!("Failed to build request: {e}"));
            let resp = handle_request(req)
                .await
                .unwrap_or_else(|e| panic!("handle_request failed for {path}: {e}"));
            assert_eq!(
                resp.status(),
                StatusCode::OK,
                "Expected 200 for {path} even when semaphore is exhausted"
            );
        }

        // Release one permit — request should now get through (not 429).
        drop(permit1);
        let resp = handle_request(services_request())
            .await
            .unwrap_or_else(|e| panic!("handle_request failed: {e}"));
        assert_ne!(
            resp.status(),
            StatusCode::TOO_MANY_REQUESTS,
            "Should not get 429 when a permit is available"
        );

        drop(permit2);
    }

    #[tokio::test]
    async fn test_privileged_logs_route_needs_flag() {
        let req = Request::builder()
            .method(Method::POST)
            .uri("/privileged_logs/open")
            .body(Full::new(Bytes::from(r#"{"path":"/var/log/syslog"}"#)))
            .unwrap_or_else(|e| panic!("Failed to build request: {e}"));
        let resp = handle_request(req)
            .await
            .unwrap_or_else(|e| panic!("handle_request failed: {e}"));
        assert_eq!(resp.status(), StatusCode::NOT_FOUND);
    }

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
