// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::lifecycle::Lifecycle;
use super::{ExitEvent, PendingRestart, ProcessManager};
use crate::command::Command;
use log::warn;
use std::future::Future;
use std::pin::Pin;
use tokio::sync::mpsc;

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
    async fn event_loop_runs_until_shutdown_signaled() {
        let _guard = platform::test_shutdown_lock().await;
        platform::reset_shutdown_state_for_test();

        let manager = empty_manager();
        let (ctx, mut rx) = running_runtime();

        startup::run(&manager, &ctx).await;
        assert!(!ctx.lifecycle.is_stopping());

        let shutdown = platform::shutdown_signal();
        tokio::pin!(shutdown);
        tokio::spawn(async {
            tokio::time::sleep(Duration::from_millis(10)).await;
            platform::signal_shutdown_for_test();
        });

        rx.run_with(&manager, &ctx, shutdown).await;

        assert!(ctx.lifecycle.is_stopping());
        platform::reset_shutdown_state_for_test();
    }
}
