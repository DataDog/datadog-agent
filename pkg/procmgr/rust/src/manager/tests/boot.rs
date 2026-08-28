// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::super::startup;
use super::super::*;
use super::{auto_start_for_test, loader, sleep_def, startup_runtime_context, uuid_gen};
use crate::config::{ProcessConfig, ProcessDefinition, RestartPolicy};
use crate::state::ProcessState;
use std::time::Duration;

#[tokio::test]
async fn test_spawn_failure_schedules_on_failure_restart() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(
        loader(vec![ProcessDefinition {
            name: "bad-spawn".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-spawn-fail".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.05),
                ..Default::default()
            },
        }]),
        uuid_gen(),
    );
    let (_lifecycle, handles, mut rx) = startup_runtime_context();

    auto_start_for_test(&mgr, &handles).await;

    assert!(!mgr.processes().await[0].is_running());
    let expected_uuid = mgr.processes().await[0].uuid().to_owned();
    let pending = tokio::time::timeout(std::time::Duration::from_secs(1), rx.restart_rx.recv())
        .await
        .expect("timed out waiting for restart after spawn failure");
    assert_eq!(
        pending.as_ref().map(|p| p.uuid.as_str()),
        Some(expected_uuid.as_str())
    );
    Ok(())
}

#[tokio::test]
async fn test_auto_start_all_skips_remaining_children_after_shutdown() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let mgr = ProcessManager::new(
        loader(vec![sleep_def("first-child"), sleep_def("second-child")]),
        uuid_gen(),
    );
    let (lifecycle, handles, _rx) = startup_runtime_context();

    let mgr_task = mgr.clone();
    let handles_task = handles.clone();
    let auto_start_task = tokio::spawn(async move {
        let shutdown = crate::platform::shutdown_signal();
        tokio::pin!(shutdown);
        startup::run(&mgr_task, &handles_task, shutdown.as_mut()).await
    });

    tokio::time::timeout(Duration::from_secs(5), async {
        loop {
            let procs = mgr.processes().await;
            if procs
                .iter()
                .any(|p| p.name() == "first-child" && p.state() == ProcessState::Running)
            {
                return;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    })
    .await
    .expect("timed out waiting for first auto-start child");

    crate::platform::signal_shutdown_for_test();
    auto_start_task.await?;

    assert!(lifecycle.is_stopping());
    let procs = mgr.processes().await;
    let first = procs
        .iter()
        .find(|p| p.name() == "first-child")
        .expect("first-child");
    let second = procs
        .iter()
        .find(|p| p.name() == "second-child")
        .expect("second-child");
    assert!(first.is_running());
    assert!(
        !second.is_running(),
        "second-child should not auto-start after shutdown is signaled"
    );

    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_auto_start_all_releases_catalog_lock_on_shutdown() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let mgr = ProcessManager::new(
        loader(vec![sleep_def("first-child"), sleep_def("second-child")]),
        uuid_gen(),
    );
    let (_lifecycle, handles, _rx) = startup_runtime_context();

    let mgr_task = mgr.clone();
    let handles_task = handles.clone();
    let auto_start_task = tokio::spawn(async move {
        let shutdown = crate::platform::shutdown_signal();
        tokio::pin!(shutdown);
        startup::run(&mgr_task, &handles_task, shutdown.as_mut()).await
    });

    tokio::time::timeout(Duration::from_secs(5), async {
        loop {
            let procs = mgr.processes().await;
            if procs
                .iter()
                .any(|p| p.name() == "first-child" && p.state() == ProcessState::Running)
            {
                return;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    })
    .await
    .expect("timed out waiting for first auto-start child");

    crate::platform::signal_shutdown_for_test();
    auto_start_task.await?;

    let _write_guard =
        tokio::time::timeout(Duration::from_millis(100), mgr.catalog.write_processes())
            .await
            .expect("catalog write lock still held after auto-start shutdown join");

    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_shutdown_during_auto_start_transitions_lifecycle() {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let mgr = ProcessManager::new(
        loader(vec![sleep_def("first-child"), sleep_def("second-child")]),
        uuid_gen(),
    );
    let (lifecycle, handles, _rx) = startup_runtime_context();

    crate::platform::signal_shutdown_for_test();
    let pending = std::future::pending::<()>();
    tokio::pin!(pending);
    startup::run(&mgr, &handles, pending.as_mut()).await;

    assert!(lifecycle.is_stopping());
    crate::platform::reset_shutdown_state_for_test();
}
