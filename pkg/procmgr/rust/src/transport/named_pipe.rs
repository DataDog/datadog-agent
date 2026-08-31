// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context as _, Result};
use log::info;
use std::ffi::OsString;
use std::future::Future;
use std::io;
use std::os::windows::io::AsRawHandle;
use std::path::{Path, PathBuf};
use std::pin::Pin;
use std::sync::{Arc, OnceLock};
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};
use tokio::net::windows::named_pipe::{NamedPipeServer, ServerOptions};
use windows_sys::Win32::Foundation::HANDLE;

use crate::platform::{create_pipe_server, pipe_client_may_mutate};

const DEFAULT_PIPE_INSTANCES: usize = 4;

pub fn ipc_path() -> PathBuf {
    dd_procmgr_client::ipc_path()
}

pub fn prepare(_path: &Path) -> Result<()> {
    Ok(())
}

pub fn set_permissions(_path: &Path) {}

pub fn cleanup(_path: &Path) {}

#[derive(Clone)]
pub struct PipeCallerAuth {
    pipe: PipeHandle,
    may_mutate: Arc<OnceLock<bool>>,
}

#[derive(Clone, Copy, Debug)]
struct PipeHandle(HANDLE);

unsafe impl Send for PipeHandle {}
unsafe impl Sync for PipeHandle {}

impl PipeCallerAuth {
    fn new(pipe: &NamedPipeServer) -> Self {
        Self {
            pipe: PipeHandle(pipe.as_raw_handle() as HANDLE),
            may_mutate: Arc::new(OnceLock::new()),
        }
    }

    pub fn may_mutate(&self) -> bool {
        *self
            .may_mutate
            .get_or_init(|| pipe_client_may_mutate(self.pipe.0))
    }
}

/// Newtype around [`NamedPipeServer`] that implements
/// [`tonic::transport::server::Connected`] so tonic can serve over it.
struct NamedPipeIo {
    pipe: NamedPipeServer,
    caller: PipeCallerAuth,
}

impl NamedPipeIo {
    fn new(pipe: NamedPipeServer) -> Self {
        let caller = PipeCallerAuth::new(&pipe);
        Self { pipe, caller }
    }
}

impl tonic::transport::server::Connected for NamedPipeIo {
    type ConnectInfo = PipeCallerAuth;

    fn connect_info(&self) -> Self::ConnectInfo {
        self.caller.clone()
    }
}

impl AsyncRead for NamedPipeIo {
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        Pin::new(&mut self.pipe).poll_read(cx, buf)
    }
}

impl AsyncWrite for NamedPipeIo {
    fn poll_write(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<io::Result<usize>> {
        Pin::new(&mut self.pipe).poll_write(cx, buf)
    }

    fn poll_flush(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.pipe).poll_flush(cx)
    }

    fn poll_shutdown(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.pipe).poll_shutdown(cx)
    }
}

pub async fn serve<F>(router: tonic::transport::server::Router, shutdown: F) -> Result<()>
where
    F: Future<Output = ()>,
{
    serve_at_path(router, &ipc_path(), shutdown).await
}

pub(crate) async fn serve_at_path<F>(
    router: tonic::transport::server::Router,
    path: &Path,
    shutdown: F,
) -> Result<()>
where
    F: Future<Output = ()>,
{
    let pipe_name = path.as_os_str().to_os_string();

    let mut server_options = ServerOptions::new();
    server_options.first_pipe_instance(true);
    let server =
        create_pipe_server(&server_options, &pipe_name).context("failed to create named pipe")?;

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

    // Cancel the accept loop before returning so we do not leak a task blocked on connect().
    accept_handle.abort();

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

/// Accept connections on the named pipe and create a new instance after each connect.
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
        server = create_pipe_server(&ServerOptions::new(), &pipe_name)
            .context("failed to create next named pipe instance")?;

        let io = NamedPipeIo::new(connected);

        if tx.send(Ok(io)).await.is_err() {
            break;
        }
    }

    Ok(())
}
