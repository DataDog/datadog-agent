// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::lifecycle::Lifecycle;
use super::{ExitEvent, PendingRestart, ProcessManager};
use crate::command::Command;
use log::warn;
use std::future::Future;
use std::pin::{Pin, pin};
use tokio::sync::mpsc;
use tonic::Status;

#[derive(Clone)]
pub(crate) struct RuntimeContext {
    pub(crate) cmd_tx: mpsc::Sender<Command>,
    pub(in crate::manager) exit_tx: mpsc::Sender<ExitEvent>,
    pub(in crate::manager) restart_tx: mpsc::Sender<PendingRestart>,
    pub(in crate::manager) lifecycle: Lifecycle,
}

pub(crate) struct RuntimeReceivers {
    pub(in crate::manager) cmd_rx: mpsc::Receiver<Command>,
    pub(in crate::manager) exit_rx: mpsc::Receiver<ExitEvent>,
    pub(in crate::manager) restart_rx: mpsc::Receiver<PendingRestart>,
}

impl RuntimeContext {
    fn with_senders(
        lifecycle: Lifecycle,
        cmd_tx: mpsc::Sender<Command>,
        cmd_rx: mpsc::Receiver<Command>,
    ) -> (Self, RuntimeReceivers) {
        let (exit_tx, exit_rx) = mpsc::channel(256);
        let (restart_tx, restart_rx) = mpsc::channel(256);
        (
            Self {
                cmd_tx,
                exit_tx,
                restart_tx,
                lifecycle,
            },
            RuntimeReceivers {
                cmd_rx,
                exit_rx,
                restart_rx,
            },
        )
    }

    pub(super) fn new(lifecycle: Lifecycle) -> (Self, RuntimeReceivers) {
        let (cmd_tx, cmd_rx) = mpsc::channel(64);
        Self::with_senders(lifecycle, cmd_tx, cmd_rx)
    }

    #[cfg(all(test, unix))]
    pub(super) fn with_cmd(
        lifecycle: Lifecycle,
        cmd_tx: mpsc::Sender<Command>,
        cmd_rx: mpsc::Receiver<Command>,
    ) -> (Self, RuntimeReceivers) {
        Self::with_senders(lifecycle, cmd_tx, cmd_rx)
    }
}

impl RuntimeReceivers {
    pub(in crate::manager) async fn drain_pending_commands(&mut self) {
        let status = Status::unavailable("dd-procmgrd is shutting down");
        while let Ok(cmd) = self.cmd_rx.try_recv() {
            cmd.reject(status.clone());
        }
    }

    pub(in crate::manager) async fn run_with(
        &mut self,
        manager: &ProcessManager,
        ctx: &RuntimeContext,
        mut shutdown: Pin<&mut impl Future<Output = ()>>,
    ) {
        loop {
            tokio::select! {
                _ = shutdown.as_mut() => {
                    ctx.lifecycle.begin_stopping();
                    return;
                }
                Some(cmd) = self.cmd_rx.recv() => {
                    let manager = manager.clone();
                    let ctx = ctx.clone();
                    tokio::spawn(async move {
                        manager.handle_command(&ctx, cmd).await;
                    });
                }
                Some(event) = self.exit_rx.recv() => {
                    manager.handle_exit(event, ctx).await;
                }
                Some(pending) = self.restart_rx.recv() => {
                    manager.complete_restart(pending, ctx).await;
                }
                else => {
                    warn!("manager event loop exiting: all channels closed");
                    ctx.lifecycle.begin_stopping();
                    return;
                }
            }
        }
    }

    pub(in crate::manager) async fn drain_exits_during(
        &mut self,
        manager: &ProcessManager,
        ctx: &RuntimeContext,
        work: impl Future<Output = ()>,
    ) {
        let mut work = pin!(work);
        loop {
            tokio::select! {
                biased;
                _ = work.as_mut() => break,
                Some(_event) = self.exit_rx.recv() => {}
            }
        }
        self.drain_pending_exits(manager, ctx).await;
    }

    async fn drain_pending_exits(&mut self, manager: &ProcessManager, ctx: &RuntimeContext) {
        while let Ok(event) = self.exit_rx.try_recv() {
            manager.handle_exit(event, ctx).await;
        }
        while let Ok(Some(event)) =
            tokio::time::timeout(Self::EXIT_DRAIN_IDLE, self.exit_rx.recv()).await
        {
            manager.handle_exit(event, ctx).await;
        }
    }

    const EXIT_DRAIN_IDLE: std::time::Duration = std::time::Duration::from_millis(100);
}

#[cfg(all(test, unix))]
pub(crate) fn spawn_command_loop_for_tests(
    manager: ProcessManager,
    cmd_tx: mpsc::Sender<Command>,
    cmd_rx: mpsc::Receiver<Command>,
) {
    let lifecycle = Lifecycle::new();
    lifecycle.begin_running();
    let (ctx, mut rx) = RuntimeContext::with_cmd(lifecycle, cmd_tx, cmd_rx);
    tokio::spawn(async move {
        let pending = std::future::pending::<()>();
        tokio::pin!(pending);
        rx.run_with(&manager, &ctx, pending).await;
    });
}

#[cfg(test)]
mod tests {
    use super::super::startup;
    use super::*;
    use crate::config::StaticConfigLoader;
    use crate::platform;
    use crate::uuid_gen::V4UuidGenerator;
    use std::sync::Arc;
    use std::time::Duration;

    fn empty_manager() -> ProcessManager {
        ProcessManager::new(
            Arc::new(StaticConfigLoader::new(vec![])),
            Arc::new(V4UuidGenerator),
        )
    }

    fn running_runtime() -> (RuntimeContext, RuntimeReceivers) {
        let lifecycle = Lifecycle::new();
        lifecycle.begin_running();
        RuntimeContext::new(lifecycle)
    }

    #[tokio::test]
    async fn drain_pending_commands_rejects_queued_mutations() {
        use crate::command::Command;
        use tokio::sync::{mpsc, oneshot};

        let (cmd_tx, cmd_rx) = mpsc::channel(64);
        let lifecycle = Lifecycle::new();
        lifecycle.begin_stopping();
        let (_ctx, mut rx) = RuntimeContext::with_cmd(lifecycle, cmd_tx.clone(), cmd_rx);

        let (reply_tx, reply_rx) = oneshot::channel();
        cmd_tx
            .send(Command::Start {
                name_or_uuid: "svc".to_string(),
                reply: reply_tx,
            })
            .await
            .expect("command send should succeed");

        rx.drain_pending_commands().await;

        let err = reply_rx
            .await
            .expect("reply channel should receive rejection")
            .expect_err("queued command should be rejected during shutdown");
        assert_eq!(err.code(), tonic::Code::Unavailable);
    }

    #[tokio::test]
    async fn event_loop_runs_until_shutdown_signaled() {
        let _guard = platform::test_shutdown_lock().await;
        platform::reset_shutdown_state_for_test();

        let manager = empty_manager();
        let (ctx, mut rx) = running_runtime();

        let shutdown = platform::shutdown_signal();
        tokio::pin!(shutdown);
        startup::run(&manager, &ctx, shutdown.as_mut()).await;
        assert!(!ctx.lifecycle.is_stopping());

        tokio::spawn(async {
            tokio::time::sleep(Duration::from_millis(10)).await;
            platform::signal_shutdown_for_test();
        });

        rx.run_with(&manager, &ctx, shutdown).await;

        assert!(ctx.lifecycle.is_stopping());
        platform::reset_shutdown_state_for_test();
    }

    #[tokio::test]
    async fn drain_exits_during_work_drains_beyond_channel_capacity() {
        let manager = empty_manager();
        let (ctx, mut rx) = running_runtime();

        let senders: Vec<_> = (0..300)
            .map(|i| {
                let tx = ctx.exit_tx.clone();
                tokio::spawn(async move {
                    tx.send(ExitEvent {
                        name: format!("svc-{i}"),
                        pid: i as u32,
                        status: crate::test_helpers::exit_status(0),
                    })
                    .await
                    .expect("exit send should not block indefinitely");
                })
            })
            .collect();

        tokio::time::timeout(
            Duration::from_secs(5),
            rx.drain_exits_during(&manager, &ctx, async {
                tokio::time::sleep(Duration::from_millis(50)).await;
            }),
        )
        .await
        .expect("drain should complete while exit senders are active");

        for sender in senders {
            sender.await.expect("exit sender task should complete");
        }
    }

    #[tokio::test]
    async fn drain_exits_during_work_drains_while_catalog_write_locked() {
        let manager = empty_manager();
        let (ctx, mut rx) = running_runtime();
        let catalog = Arc::clone(&manager.catalog);

        let senders: Vec<_> = (0..300)
            .map(|i| {
                let tx = ctx.exit_tx.clone();
                tokio::spawn(async move {
                    tx.send(ExitEvent {
                        name: format!("svc-{i}"),
                        pid: i as u32,
                        status: crate::test_helpers::exit_status(0),
                    })
                    .await
                    .expect("exit send should not block while catalog lock is held");
                })
            })
            .collect();

        tokio::time::timeout(
            Duration::from_secs(5),
            rx.drain_exits_during(&manager, &ctx, async {
                let _guard = catalog.write_processes().await;
                tokio::time::sleep(Duration::from_millis(100)).await;
            }),
        )
        .await
        .expect("drain should keep receiving while shutdown holds the catalog lock");

        for sender in senders {
            sender.await.expect("exit sender task should complete");
        }
    }
}
