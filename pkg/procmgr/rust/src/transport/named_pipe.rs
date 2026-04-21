// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context as _, Result};
use log::info;
use std::ffi::OsString;
use std::future::Future;
use std::io;
use std::path::{Path, PathBuf};
use std::pin::Pin;
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};
use tokio::net::windows::named_pipe::{ClientOptions, NamedPipeServer, ServerOptions};
use windows_sys::Win32::Foundation::ERROR_PIPE_BUSY;

const DEFAULT_PIPE_PATH: &str = r"\\.\pipe\datadog-procmgrd";
const DEFAULT_PIPE_INSTANCES: usize = 4;

/// Placeholder URI for tonic Endpoint when connecting over Named Pipes.
/// The actual address is irrelevant because `connect_with_connector` bypasses it.
pub const DUMMY_ENDPOINT: &str = "http://[::]:50051";

pub fn ipc_path() -> PathBuf {
    std::env::var("DD_PM_SOCKET_PATH")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from(DEFAULT_PIPE_PATH))
}

/// Named pipes don't require filesystem preparation.
pub fn prepare(_path: &Path) -> Result<()> {
    Ok(())
}

/// Named pipe permissions are set via security descriptors at creation time.
pub fn set_permissions(_path: &Path) {}

/// Named pipes are kernel objects; no filesystem cleanup needed.
pub fn cleanup(_path: &Path) {}

// ---------------------------------------------------------------------------
// NamedPipeIo — wrapper for tonic's `Connected` trait
// ---------------------------------------------------------------------------

/// Newtype around [`NamedPipeServer`] that implements
/// [`tonic::transport::server::Connected`] so tonic can serve over it.
struct NamedPipeIo(NamedPipeServer);

impl tonic::transport::server::Connected for NamedPipeIo {
    type ConnectInfo = ();
    fn connect_info(&self) -> Self::ConnectInfo {}
}

impl AsyncRead for NamedPipeIo {
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        Pin::new(&mut self.0).poll_read(cx, buf)
    }
}

impl AsyncWrite for NamedPipeIo {
    fn poll_write(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<io::Result<usize>> {
        Pin::new(&mut self.0).poll_write(cx, buf)
    }

    fn poll_flush(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.0).poll_flush(cx)
    }

    fn poll_shutdown(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.0).poll_shutdown(cx)
    }
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

pub async fn serve<F>(router: tonic::transport::server::Router, shutdown: F) -> Result<()>
where
    F: Future<Output = ()>,
{
    let path = ipc_path();
    let pipe_name = path.as_os_str().to_os_string();

    let server = ServerOptions::new()
        .first_pipe_instance(true)
        .create(&pipe_name)
        .context("failed to create named pipe")?;

    info!("gRPC server listening on {}", path.display());

    let max_instances = std::env::var("DD_PM_PIPE_INSTANCES")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(DEFAULT_PIPE_INSTANCES);
    let (tx, rx) = tokio::sync::mpsc::channel::<io::Result<NamedPipeIo>>(max_instances);

    let accept_handle = tokio::spawn(accept_loop(pipe_name, server, tx));

    let incoming = tokio_stream::wrappers::ReceiverStream::new(rx);

    let serve_result = router
        .serve_with_incoming_shutdown(incoming, shutdown)
        .await
        .context("gRPC server error");

    // Always cancel the accept loop before returning — even on error — so we
    // don't leak a background task blocked on server.connect().
    accept_handle.abort();

    // Surface the accept-loop error when tonic returned successfully (e.g. the
    // incoming stream ended because the accept loop hit a fatal error and
    // dropped the sender).
    serve_result?;
    match accept_handle.await {
        Ok(Ok(())) => {}
        Ok(Err(e)) => return Err(e).context("named pipe accept loop failed"),
        Err(join_err) if join_err.is_cancelled() => {}
        Err(join_err) => std::panic::resume_unwind(join_err.into_panic()),
    }

    info!("gRPC server stopped");
    Ok(())
}

/// Accept connections on the named pipe, sending each connected instance
/// through the channel. Creates a new pipe instance after each connection
/// so the next client can connect.
async fn accept_loop(
    pipe_name: OsString,
    mut server: NamedPipeServer,
    tx: tokio::sync::mpsc::Sender<io::Result<NamedPipeIo>>,
) -> Result<()> {
    loop {
        if let Err(e) = server.connect().await {
            let msg = format!(
                "named pipe accept failed on {}: {}",
                pipe_name.to_string_lossy(),
                e
            );
            let _ = tx.send(Err(e)).await;
            anyhow::bail!(msg);
        }

        let connected = server;
        server = ServerOptions::new()
            .create(&pipe_name)
            .context("failed to create next named pipe instance")?;

        if tx.send(Ok(NamedPipeIo(connected))).await.is_err() {
            break;
        }
    }

    Ok(())
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

pub async fn connect(path: &Path) -> Result<tonic::transport::Channel> {
    let pipe_name = path.as_os_str().to_os_string();
    let channel = tonic::transport::Endpoint::from_static(DUMMY_ENDPOINT)
        .connect_with_connector(tower::service_fn(move |_| {
            let name = pipe_name.clone();
            async move { open_pipe_with_retry(&name).await }
        }))
        .await
        .with_context(|| format!("failed to connect to named pipe {}", path.display()))?;
    Ok(channel)
}

const PIPE_BUSY_RETRIES: u32 = 5;
const PIPE_BUSY_BACKOFF_MS: u64 = 50;

/// Open a named pipe client, retrying on `ERROR_PIPE_BUSY`.
///
/// All server instances may be occupied when the client calls `open()`.
/// Windows named pipe clients are expected to wait and retry in this case.
async fn open_pipe_with_retry(
    name: &std::ffi::OsStr,
) -> io::Result<hyper_util::rt::TokioIo<tokio::net::windows::named_pipe::NamedPipeClient>> {
    let mut backoff = PIPE_BUSY_BACKOFF_MS;
    for attempt in 0..PIPE_BUSY_RETRIES {
        match ClientOptions::new().open(name) {
            Ok(client) => return Ok(hyper_util::rt::TokioIo::new(client)),
            Err(e)
                if e.raw_os_error() == Some(ERROR_PIPE_BUSY as i32)
                    && attempt + 1 < PIPE_BUSY_RETRIES =>
            {
                tokio::time::sleep(std::time::Duration::from_millis(backoff)).await;
                backoff *= 2;
            }
            Err(e) => return Err(e),
        }
    }
    unreachable!()
}
