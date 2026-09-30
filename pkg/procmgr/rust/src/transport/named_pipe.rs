// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context as _, Result};
use log::{error, info, warn};
use std::ffi::{OsStr, OsString};
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

use super::accept_backoff::{AcceptBackoff, Retry};
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
    let path = ipc_path();
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
    // tonic requires a fallible stream, but only connections are ever sent: it answers an
    // error by tracing it and moving on, which is no way to find out that the supervisor
    // has lost its control plane, so `accept_loop` reports failures itself.
    let (tx, rx) = tokio::sync::mpsc::channel::<io::Result<NamedPipeIo>>(max_instances);

    let accept_handle = tokio::spawn(accept_loop(pipe_name, server, tx));

    let incoming = tokio_stream::wrappers::ReceiverStream::new(rx);

    let serve_result = router
        .serve_with_incoming_shutdown(incoming, shutdown)
        .await
        .context("gRPC server error");

    accept_handle.abort();

    serve_result?;
    match accept_handle.await {
        Ok(()) => {}
        Err(join_err) if join_err.is_cancelled() => {}
        Err(join_err) => std::panic::resume_unwind(join_err.into_panic()),
    }

    info!("gRPC server stopped");
    Ok(())
}

/// Hands connected pipe instances to the gRPC server until it stops taking them.
///
/// Never returns because an accept failed. This task is the supervisor's whole control
/// plane: once it returns, `serve` runs out of instances to hand over and stops
/// listening, and `manager` does not look at the outcome until the daemon shuts down, so
/// every later `dd-procmgr status`, `stop` or `reload` fails until the service is
/// restarted. A failure here is not worth that. mio already reports the benign races (a
/// client that connected before we asked, or left before we looked) as success, so what
/// reaches us is either resource pressure, which passes, or an instance in a state a
/// fresh one replaces.
async fn accept_loop(
    pipe_name: OsString,
    mut server: NamedPipeServer,
    tx: tokio::sync::mpsc::Sender<io::Result<NamedPipeIo>>,
) {
    let mut backoff = AcceptBackoff::new();

    loop {
        if let Err(e) = server.connect().await {
            let retry = backoff.record_failure();
            log_retry("accept", &pipe_name, &e, &retry);
            // Replace the instance, since a failure may have left it unusable. Assigning
            // builds the replacement before dropping the old one, which keeps the name
            // owned the whole time: with no instance open, any local process could create
            // the pipe and answer in the supervisor's place.
            server = create_pipe_instance(&pipe_name).await;
            // A client arriving during the wait is not turned away. It connects to the new
            // instance, and the `connect` below then returns straight away.
            tokio::time::sleep(retry.delay).await;
            continue;
        }
        backoff.record_success();

        let connected = server;
        if tx.send(Ok(NamedPipeIo::new(connected))).await.is_err() {
            break;
        }

        server = create_pipe_instance(&pipe_name).await;
    }
}

/// Adds an instance to the pipe, waiting out failures.
///
/// `ServerOptions::new` deliberately leaves `first_pipe_instance` unset: only the
/// instance `serve` creates may claim it, and claiming it here would fail against the
/// instances already open.
async fn create_pipe_instance(pipe_name: &OsStr) -> NamedPipeServer {
    let mut backoff = AcceptBackoff::new();

    loop {
        match create_pipe_server(&ServerOptions::new(), pipe_name) {
            Ok(server) => return server,
            Err(e) => {
                let retry = backoff.record_failure();
                log_retry("instance creation", pipe_name, &e, &retry);
                tokio::time::sleep(retry.delay).await;
            }
        }
    }
}

fn log_retry(op: &str, pipe_name: &OsStr, e: &io::Error, retry: &Retry) {
    let name = pipe_name.to_string_lossy();
    if retry.persistent {
        error!(
            "named pipe {op} on {name} has failed {} times running, most recently: {e:?}. IPC is unavailable until this clears",
            retry.consecutive
        );
    } else {
        warn!(
            "named pipe {op} failed on {name}: {e:?}; retrying in {:?}",
            retry.delay
        );
    }
}
