// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::lifecycle::Lifecycle;
use super::runtime::RuntimeReceivers;
use super::startup;
use super::*;
use crate::config::{ConfigLoader, ProcessConfig, ProcessDefinition, StaticConfigLoader};
use crate::test_helpers;
use crate::uuid_gen::{UuidGenerator, V4UuidGenerator};
use std::sync::Arc;
use std::time::Duration;

pub fn loader(defs: Vec<ProcessDefinition>) -> Arc<dyn ConfigLoader> {
    Arc::new(StaticConfigLoader::new(defs))
}

pub fn uuid_gen() -> Arc<dyn UuidGenerator> {
    Arc::new(V4UuidGenerator)
}

pub fn test_runtime_context() -> (RuntimeContext, RuntimeReceivers) {
    let lifecycle = Lifecycle::new();
    lifecycle.begin_running();
    RuntimeContext::new(lifecycle)
}

pub fn startup_runtime_context() -> (Lifecycle, RuntimeContext, RuntimeReceivers) {
    let lifecycle = Lifecycle::new();
    let (ctx, rx) = RuntimeContext::new(lifecycle.clone());
    (lifecycle, ctx, rx)
}

pub async fn auto_start_for_test(mgr: &ProcessManager, ctx: &RuntimeContext) {
    let _guard = crate::platform::test_shutdown_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let pending = std::future::pending::<()>();
    tokio::pin!(pending);
    startup::run(mgr, ctx, pending.as_mut()).await;
}

pub fn current_pending_restart(proc: &ManagedProcess) -> PendingRestart {
    PendingRestart {
        uuid: proc.uuid().to_owned(),
        spawn_seq: proc.spawn_seq(),
    }
}

pub async fn wait_until_running(mgr: &ProcessManager, name: &str) {
    tokio::time::timeout(Duration::from_secs(5), async {
        loop {
            if mgr
                .processes()
                .await
                .iter()
                .any(|p| p.name() == name && p.is_running())
            {
                return;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    })
    .await
    .unwrap_or_else(|_| panic!("timed out waiting for '{name}' to start"));
}

pub fn sleep_def(name: &str) -> ProcessDefinition {
    sleep_def_secs(name, 60)
}

fn sleep_def_secs(name: &str, secs: u32) -> ProcessDefinition {
    let (cmd, args) = test_helpers::sleep_cmd(secs);
    ProcessDefinition {
        name: name.to_string(),
        config: ProcessConfig {
            command: cmd.to_string(),
            args,
            ..Default::default()
        },
    }
}

mod boot;
mod create;
mod resolve;
mod restart;
