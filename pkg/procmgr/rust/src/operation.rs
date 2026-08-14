// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::sync::Arc;

use tokio::sync::Mutex;
use tonic::Status;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum OperationKind {
    ReloadConfig,
    Shutdown,
}

#[derive(Clone, Default)]
pub(crate) struct OperationGate {
    active: Arc<Mutex<Option<OperationKind>>>,
}

impl OperationGate {
    pub async fn try_begin(&self, op: OperationKind) -> Result<(), Status> {
        let mut guard = self.active.lock().await;
        if let Some(active) = *guard {
            return Err(Status::failed_precondition(format!(
                "operation in progress ({active:?}); try again later"
            )));
        }
        *guard = Some(op);
        Ok(())
    }

    pub async fn force_begin(&self, op: OperationKind) {
        *self.active.lock().await = Some(op);
    }

    pub async fn end(&self, op: OperationKind) {
        let mut guard = self.active.lock().await;
        if *guard == Some(op) {
            *guard = None;
        }
    }

    pub async fn ensure_idle(&self) -> Result<(), Status> {
        let guard = self.active.lock().await;
        if let Some(active) = *guard {
            return Err(Status::failed_precondition(format!(
                "operation in progress ({active:?}); try again later"
            )));
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_operation_gate_blocks_while_active() {
        let gate = OperationGate::default();
        gate.try_begin(OperationKind::ReloadConfig)
            .await
            .expect("first begin");
        assert!(gate.ensure_idle().await.is_err());
        assert!(gate.try_begin(OperationKind::ReloadConfig).await.is_err());
        gate.end(OperationKind::ReloadConfig).await;
        gate.ensure_idle().await.expect("idle after end");
    }

    #[tokio::test]
    async fn test_force_begin_shutdown_replaces_active_operation() {
        let gate = OperationGate::default();
        gate.try_begin(OperationKind::ReloadConfig)
            .await
            .expect("reload begin");
        gate.force_begin(OperationKind::Shutdown).await;
        assert!(gate.ensure_idle().await.is_err());
        assert!(gate.try_begin(OperationKind::ReloadConfig).await.is_err());
    }
}
