// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::lifecycle::Lifecycle;
use super::runtime::RuntimeContext;
use super::{ProcessManager, startup};
use crate::grpc;
use crate::platform;
use anyhow::Result;
use log::{info, warn};
use std::pin::pin;
use tokio::sync::oneshot;

pub struct Supervisor {
    manager: ProcessManager,
}

impl Supervisor {
    pub(super) fn new(manager: ProcessManager) -> Self {
        Self { manager }
    }

    pub async fn run(self) -> Result<()> {
        info!("dd-procmgrd starting");

        let (ctx, mut rx) = RuntimeContext::new(Lifecycle::new());
        let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
        let grpc_handle = tokio::spawn(grpc::server::run(
            self.manager.clone(),
            ctx.clone(),
            grpc_shutdown_rx,
        ));

        let shutdown = platform::shutdown_signal();
        let mut shutdown = pin!(shutdown);
        startup::run(&self.manager, &ctx, shutdown.as_mut()).await;

        if !ctx.lifecycle.is_stopping() {
            ctx.lifecycle.begin_running();
            info!("dd-procmgrd ready");
            rx.run_with(&self.manager, &ctx, shutdown).await;
        }

        info!("dd-procmgrd shutting down");

        let _ = grpc_shutdown_tx.send(());
        match grpc_handle.await {
            Ok(Err(e)) => warn!("gRPC server error: {e}"),
            Err(e) => warn!("gRPC server task panicked: {e}"),
            Ok(Ok(())) => {}
        }

        self.manager.shutdown().await;
        info!("dd-procmgrd stopped");
        Ok(())
    }
}
