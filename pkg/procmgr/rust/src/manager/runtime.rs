// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::lifecycle::Lifecycle;
use super::tracked_join::{TrackedJoinTimeout, join_tracked_handle};
use super::{ExitEvent, PendingRestart, ProcessManager};
use crate::command::Command;
use crate::shutdown::ShutdownBudget;
use log::warn;
use std::future::Future;
use std::pin::{Pin, pin};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tonic::Status;

#[derive(Clone, Default)]
pub(in crate::manager) struct CommandHandlers {
    handles: Arc<Mutex<Vec<JoinHandle<()>>>>,
}

impl CommandHandlers {
    pub(in crate::manager) fn track(&self, handle: JoinHandle<()>) {
        self.handles.lock().unwrap().push(handle);
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(in crate::manager) async fn join_all(&self) {
        self.join_all_with_budget(None, ShutdownBudget::unlimited(Instant::now()))
            .await;
    }

    pub(in crate::manager) async fn join_all_with_budget(
        &self,
        catalog: Option<&super::catalog::ProcessCatalog>,
        budget: ShutdownBudget,
    ) {
        let handles = std::mem::take(&mut *self.handles.lock().unwrap());
        for handle in handles {
            if let Some(catalog) = catalog {
                catalog.finalize_orphaned_stop_waits(budget).await;
            }
            join_tracked_handle(
                handle,
                &budget,
                TrackedJoinTimeout::Abort {
                    log_label: "command handler",
                },
                log_command_handler_failed,
            )
            .await;
        }
        if let Some(catalog) = catalog {
            catalog.finalize_orphaned_stop_waits(budget).await;
        }
    }
}

#[derive(Clone, Default)]
pub(in crate::manager) struct BackgroundSpawns {
    handles: Arc<Mutex<Vec<JoinHandle<()>>>>,
}

impl BackgroundSpawns {
    pub(in crate::manager) fn track(&self, handle: JoinHandle<()>) {
        self.handles.lock().unwrap().push(handle);
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(in crate::manager) async fn join_all(&self) {
        self.join_all_with_budget(ShutdownBudget::unlimited(Instant::now()))
            .await;
    }

    pub(in crate::manager) async fn join_all_with_budget(&self, budget: ShutdownBudget) {
        let handles = std::mem::take(&mut *self.handles.lock().unwrap());
        for handle in handles {
            join_tracked_handle(
                handle,
                &budget,
                TrackedJoinTimeout::Defer {
                    log_label: "background spawn",
                },
                log_background_spawn_failed,
            )
            .await;
        }
    }
}

#[derive(Clone)]
pub(crate) struct RuntimeContext {
    pub(crate) cmd_tx: mpsc::Sender<Command>,
    pub(in crate::manager) exit_tx: mpsc::Sender<ExitEvent>,
    pub(in crate::manager) restart_tx: mpsc::Sender<PendingRestart>,
    pub(in crate::manager) lifecycle: Lifecycle,
    pub(in crate::manager) command_handlers: CommandHandlers,
    pub(in crate::manager) background_spawns: BackgroundSpawns,
}

pub(crate) struct RuntimeReceivers {
    pub(in crate::manager) cmd_rx: mpsc::Receiver<Command>,
    pub(in crate::manager) exit_rx: mpsc::Receiver<ExitEvent>,
    pub(in crate::manager) restart_rx: mpsc::Receiver<PendingRestart>,
}

impl RuntimeContext {
    pub(crate) fn lifecycle(&self) -> Lifecycle {
        self.lifecycle.clone()
    }

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
                command_handlers: CommandHandlers::default(),
                background_spawns: BackgroundSpawns::default(),
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

    #[cfg(test)]
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
        reject_pending_commands(&mut self.cmd_rx, &status);
    }

    pub(in crate::manager) async fn drain_commands_during_grpc_shutdown(
        &mut self,
        grpc_handle: &mut JoinHandle<anyhow::Result<()>>,
        budget: ShutdownBudget,
    ) {
        self.drain_pending_commands().await;
        let status = Status::unavailable("dd-procmgrd is shutting down");
        loop {
            if budget.is_bounded() {
                let cap = budget.remaining_cap(Duration::MAX);
                if cap.is_zero() {
                    warn!(
                        "gRPC shutdown budget exhausted; aborting graceful drain and rejecting queued commands"
                    );
                    reject_pending_commands(&mut self.cmd_rx, &status);
                    grpc_handle.abort();
                    return;
                }

                tokio::select! {
                    biased;
                    result = &mut *grpc_handle => {
                        reject_pending_commands(&mut self.cmd_rx, &status);
                        match result {
                            Ok(Err(e)) => warn!("gRPC server error: {e}"),
                            Err(e) => warn!("gRPC server task panicked: {e}"),
                            Ok(Ok(())) => {}
                        }
                        return;
                    }
                    cmd = self.cmd_rx.recv() => {
                        match cmd {
                            Some(cmd) => cmd.reject(status.clone()),
                            None => return,
                        }
                    }
                    _ = tokio::time::sleep(cap) => {
                        warn!(
                            "timed out waiting for gRPC shutdown ({cap:?} left in service shutdown budget); aborting graceful drain"
                        );
                        reject_pending_commands(&mut self.cmd_rx, &status);
                        grpc_handle.abort();
                        return;
                    }
                }
            } else {
                tokio::select! {
                    biased;
                    result = &mut *grpc_handle => {
                        reject_pending_commands(&mut self.cmd_rx, &status);
                        match result {
                            Ok(Err(e)) => warn!("gRPC server error: {e}"),
                            Err(e) => warn!("gRPC server task panicked: {e}"),
                            Ok(Ok(())) => {}
                        }
                        return;
                    }
                    cmd = self.cmd_rx.recv() => {
                        match cmd {
                            Some(cmd) => cmd.reject(status.clone()),
                            None => return,
                        }
                    }
                }
            }
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
                    let command_handlers = ctx.command_handlers.clone();
                    command_handlers.track(tokio::spawn(async move {
                        manager.handle_command(&ctx, cmd).await;
                    }));
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

fn reject_pending_commands(cmd_rx: &mut mpsc::Receiver<Command>, status: &Status) {
    while let Ok(cmd) = cmd_rx.try_recv() {
        cmd.reject(status.clone());
    }
}

fn log_command_handler_failed(log_label: &str, error: tokio::task::JoinError) {
    warn!("{log_label} failed: {error}");
}

fn log_background_spawn_failed(log_label: &str, error: tokio::task::JoinError) {
    warn!("{log_label} failed: {error}");
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
    async fn drain_commands_during_grpc_shutdown_rejects_late_commands() {
        use crate::command::Command;
        use tokio::sync::oneshot;

        let (cmd_tx, cmd_rx) = mpsc::channel(64);
        let lifecycle = Lifecycle::new();
        lifecycle.begin_stopping();
        let (_ctx, mut rx) = RuntimeContext::with_cmd(lifecycle, cmd_tx.clone(), cmd_rx);

        let mut grpc_handle = tokio::spawn(async {
            tokio::time::sleep(Duration::from_millis(100)).await;
            Ok::<(), anyhow::Error>(())
        });

        let (reply_tx, reply_rx) = oneshot::channel();
        tokio::spawn(async move {
            tokio::time::sleep(Duration::from_millis(10)).await;
            let _ = cmd_tx
                .send(Command::Start {
                    name_or_uuid: "svc".to_string(),
                    reply: reply_tx,
                })
                .await;
        });

        rx.drain_commands_during_grpc_shutdown(
            &mut grpc_handle,
            ShutdownBudget::unlimited(Instant::now()),
        )
        .await;

        let err = reply_rx
            .await
            .expect("reply channel should receive rejection")
            .expect_err("late command should be rejected while gRPC shuts down");
        assert_eq!(err.code(), tonic::Code::Unavailable);
    }

    #[tokio::test]
    async fn drain_commands_during_grpc_shutdown_respects_budget() {
        let (cmd_tx, cmd_rx) = mpsc::channel(64);
        let lifecycle = Lifecycle::new();
        lifecycle.begin_stopping();
        let (_ctx, mut rx) = RuntimeContext::with_cmd(lifecycle, cmd_tx, cmd_rx);

        let mut grpc_handle = tokio::spawn(async {
            tokio::time::sleep(Duration::from_secs(60)).await;
            Ok::<(), anyhow::Error>(())
        });

        let budget = ShutdownBudget::with_deadline(
            Instant::now(),
            Instant::now() + Duration::from_millis(50),
        );
        let started = Instant::now();
        rx.drain_commands_during_grpc_shutdown(&mut grpc_handle, budget)
            .await;
        assert!(
            started.elapsed() < Duration::from_millis(200),
            "grpc drain should return within the service shutdown budget, took {:?}",
            started.elapsed()
        );
    }

    #[tokio::test]
    async fn command_handlers_join_all_with_budget_times_out() {
        use tokio::sync::oneshot;

        let command_handlers = CommandHandlers::default();
        let (release_tx, release_rx) = oneshot::channel::<()>();
        command_handlers.track(tokio::spawn(async move {
            let _ = release_rx.await;
        }));

        let budget = ShutdownBudget::with_deadline(
            Instant::now(),
            Instant::now() + Duration::from_millis(50),
        );
        let mut join_task = tokio::spawn({
            let command_handlers = command_handlers.clone();
            async move {
                command_handlers.join_all_with_budget(None, budget).await;
            }
        });

        tokio::time::timeout(Duration::from_millis(200), &mut join_task)
            .await
            .expect("join_all_with_budget should return before blocked handler completes")
            .expect("join task should succeed");

        assert!(
            release_tx.send(()).is_err(),
            "timed-out command handler should be aborted instead of left running"
        );
    }

    #[tokio::test]
    async fn command_handlers_join_waits_for_in_flight_handlers() {
        use tokio::sync::oneshot;

        let command_handlers = CommandHandlers::default();
        let (started_tx, started_rx) = oneshot::channel();
        let (release_tx, release_rx) = oneshot::channel::<()>();

        command_handlers.track(tokio::spawn(async move {
            let _ = started_tx.send(());
            let _ = release_rx.await;
        }));

        started_rx.await.expect("handler should start");

        let mut join_task = tokio::spawn({
            let command_handlers = command_handlers.clone();
            async move {
                command_handlers.join_all().await;
            }
        });

        assert!(
            tokio::time::timeout(Duration::from_millis(50), &mut join_task)
                .await
                .is_err(),
            "join_all should wait for in-flight command handlers"
        );

        drop(release_tx);
        join_task
            .await
            .expect("join task should complete after handler releases");
    }

    #[tokio::test]
    async fn background_spawns_join_all_with_budget_times_out() {
        let background_spawns = BackgroundSpawns::default();
        let (release_tx, release_rx) = tokio::sync::oneshot::channel::<()>();
        background_spawns.track(tokio::spawn(async move {
            let _ = release_rx.await;
        }));

        let budget = ShutdownBudget::with_deadline(
            Instant::now(),
            Instant::now() + Duration::from_millis(50),
        );
        let mut join_task = tokio::spawn({
            let background_spawns = background_spawns.clone();
            async move {
                background_spawns.join_all_with_budget(budget).await;
            }
        });

        tokio::time::timeout(Duration::from_millis(200), &mut join_task)
            .await
            .expect("join_all_with_budget should return before blocked task completes")
            .expect("join task should succeed");

        release_tx
            .send(())
            .expect("deferred background task should still be running");
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
