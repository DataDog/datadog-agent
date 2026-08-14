use super::{ExitEvent, PendingRestart, ProcessManager};
use crate::command::Command;
use crate::grpc;
use crate::platform;
use anyhow::Result;
use log::{info, warn};
use std::future::Future;
use std::pin::Pin;
use tokio::sync::{mpsc, oneshot};

/// Cloneable senders passed to spawn/restart handlers during a supervisor run.
#[derive(Clone)]
pub(crate) struct RuntimeHandles {
    exit_tx: mpsc::Sender<ExitEvent>,
    restart_tx: mpsc::Sender<PendingRestart>,
}

impl RuntimeHandles {
    pub(super) fn new() -> (Self, mpsc::Receiver<ExitEvent>, mpsc::Receiver<PendingRestart>) {
        let (exit_tx, exit_rx) = mpsc::channel(256);
        let (restart_tx, restart_rx) = mpsc::channel(256);
        (
            Self {
                exit_tx,
                restart_tx,
            },
            exit_rx,
            restart_rx,
        )
    }

    pub(super) fn exit_sender(&self) -> mpsc::Sender<ExitEvent> {
        self.exit_tx.clone()
    }

    pub(super) fn restart_sender(&self) -> mpsc::Sender<PendingRestart> {
        self.restart_tx.clone()
    }
}

async fn handle_command(manager: &ProcessManager, handles: &RuntimeHandles, cmd: Command) {
    match cmd {
        Command::Create { name, config, reply } => {
            let _ = reply.send(manager.handle_create(name, *config, handles).await);
        }
        Command::Start { name_or_uuid, reply } => {
            let _ = reply.send(manager.handle_start(&name_or_uuid, handles).await);
        }
        Command::Stop { name_or_uuid, reply } => {
            let _ = reply.send(manager.handle_stop(&name_or_uuid).await);
        }
        Command::ReloadConfig { reply } => {
            let _ = reply.send(manager.handle_reload_config(handles).await);
        }
    }
}

/// Returns `true` when the loop exits due to shutdown, `false` when all channels close.
pub(super) async fn run_manager_event_loop(
    manager: &ProcessManager,
    handles: &RuntimeHandles,
    cmd_rx: &mut mpsc::Receiver<Command>,
    exit_rx: &mut mpsc::Receiver<ExitEvent>,
    restart_rx: &mut mpsc::Receiver<PendingRestart>,
    mut shutdown: Pin<&mut impl Future<Output = ()>>,
) -> bool {
    loop {
        tokio::select! {
            _ = shutdown.as_mut() => return true,
            Some(cmd) = cmd_rx.recv() => {
                handle_command(manager, handles, cmd).await;
            }
            Some(event) = exit_rx.recv() => {
                manager.handle_exit(event, handles).await;
            }
            Some(pending) = restart_rx.recv() => {
                manager.complete_restart(pending, handles).await;
            }
            else => return false,
        }
    }
}

/// Event loop wiring around a [`ProcessManager`] (gRPC, exit/restart channels, shutdown).
pub struct Supervisor {
    manager: ProcessManager,
}

impl Supervisor {
    pub(super) fn new(manager: ProcessManager) -> Self {
        Self { manager }
    }

    /// Run the daemon until a platform shutdown signal is received.
    pub async fn run(self) -> Result<()> {
        let manager = self.manager;
        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Command>(64);
        let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
        let grpc_handle =
            tokio::spawn(grpc::server::run(manager.clone(), cmd_tx, grpc_shutdown_rx));

        let (handles, mut exit_rx, mut restart_rx) = RuntimeHandles::new();
        manager.start_configured_processes(&handles).await;

        let shutdown = platform::shutdown_signal();
        tokio::pin!(shutdown);
        run_manager_event_loop(
            &manager,
            &handles,
            &mut cmd_rx,
            &mut exit_rx,
            &mut restart_rx,
            shutdown,
        )
        .await;

        info!("dd-procmgrd shutting down");

        let _ = grpc_shutdown_tx.send(());
        match grpc_handle.await {
            Ok(Err(e)) => warn!("gRPC server error: {e}"),
            Err(e) => warn!("gRPC server task panicked: {e}"),
            Ok(Ok(())) => {}
        }

        manager.shutdown().await;
        info!("dd-procmgrd stopped");
        Ok(())
    }
}

#[cfg(test)]
pub(crate) fn spawn_command_loop_for_tests(
    manager: ProcessManager,
    mut cmd_rx: mpsc::Receiver<Command>,
) {
    let (handles, mut exit_rx, mut restart_rx) = RuntimeHandles::new();
    tokio::spawn(async move {
        let pending = std::future::pending::<()>();
        tokio::pin!(pending);
        let _ = run_manager_event_loop(
            &manager,
            &handles,
            &mut cmd_rx,
            &mut exit_rx,
            &mut restart_rx,
            pending,
        )
        .await;
    });
}
