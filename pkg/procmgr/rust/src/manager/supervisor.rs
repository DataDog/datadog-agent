use super::{ExitEvent, PendingRestart, ProcessManager};
use crate::command::Command;
use crate::grpc;
use crate::platform;
use anyhow::Result;
use log::{info, warn};
use std::future::Future;
use std::pin::Pin;
use tokio::sync::{mpsc, oneshot};

#[derive(Clone)]
pub(crate) struct RuntimeHandles {
    pub(in crate::manager) exit_tx: mpsc::Sender<ExitEvent>,
    pub(in crate::manager) restart_tx: mpsc::Sender<PendingRestart>,
}

impl RuntimeHandles {
    pub(super) fn new() -> (
        Self,
        mpsc::Receiver<ExitEvent>,
        mpsc::Receiver<PendingRestart>,
    ) {
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
}

async fn handle_command(manager: &ProcessManager, handles: &RuntimeHandles, cmd: Command) {
    match cmd {
        Command::Create {
            name,
            config,
            reply,
        } => {
            let _ = reply.send(manager.handle_create(name, *config, handles).await);
        }
        Command::Start {
            name_or_uuid,
            reply,
        } => {
            let _ = reply.send(manager.handle_start(&name_or_uuid, handles).await);
        }
        Command::Stop {
            name_or_uuid,
            reply,
        } => {
            let _ = reply.send(manager.handle_stop(&name_or_uuid).await);
        }
    }
}

pub(super) async fn run_manager_event_loop(
    manager: &ProcessManager,
    handles: &RuntimeHandles,
    cmd_rx: &mut mpsc::Receiver<Command>,
    exit_rx: &mut mpsc::Receiver<ExitEvent>,
    restart_rx: &mut mpsc::Receiver<PendingRestart>,
    mut shutdown: Pin<&mut impl Future<Output = ()>>,
) {
    loop {
        tokio::select! {
            _ = shutdown.as_mut() => return,
            Some(cmd) = cmd_rx.recv() => {
                handle_command(manager, handles, cmd).await;
            }
            Some(event) = exit_rx.recv() => {
                manager.handle_exit(event, handles).await;
            }
            Some(pending) = restart_rx.recv() => {
                manager.complete_restart(pending, handles).await;
            }
            else => {
                warn!("manager event loop exiting: all channels closed");
                return;
            }
        }
    }
}

pub struct Supervisor {
    manager: ProcessManager,
}

impl Supervisor {
    pub(super) fn new(manager: ProcessManager) -> Self {
        Self { manager }
    }

    pub async fn run(self) -> Result<()> {
        info!("dd-procmgrd starting");

        let manager = self.manager;
        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Command>(64);
        let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
        let grpc_handle =
            tokio::spawn(grpc::server::run(manager.clone(), cmd_tx, grpc_shutdown_rx));

        let (handles, mut exit_rx, mut restart_rx) = RuntimeHandles::new();
        manager.auto_start_all(&handles).await;

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

#[cfg(all(test, unix))]
pub(crate) fn spawn_command_loop_for_tests(
    manager: ProcessManager,
    mut cmd_rx: mpsc::Receiver<Command>,
) {
    let (handles, mut exit_rx, mut restart_rx) = RuntimeHandles::new();
    tokio::spawn(async move {
        let pending = std::future::pending::<()>();
        tokio::pin!(pending);
        run_manager_event_loop(
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
