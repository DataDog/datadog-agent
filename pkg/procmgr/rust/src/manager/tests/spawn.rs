// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::super::*;
use super::{loader, sleep_def, test_runtime_context, uuid_gen, wait_until_running};
use crate::config::ProcessConfig;
use crate::state::ProcessState;
use crate::test_helpers;
use std::time::Duration;

#[tokio::test]
async fn test_stop_during_in_flight_spawn_aborts_child() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let _gate = super::super::spawn::close_spawn_gate_for_test();

    let mgr = ProcessManager::new(loader(vec![sleep_def("blocked-svc")]), uuid_gen());
    let (handles, _rx) = test_runtime_context();

    let catalog = mgr.catalog.clone();
    let ctx = handles.clone();
    let spawn_task = tokio::spawn(async move {
        super::super::spawn::spawn_process(catalog, 0, &ctx, super::super::spawn::SpawnKind::Manual)
            .await
    });

    tokio::time::timeout(Duration::from_secs(1), async {
        loop {
            let procs = mgr.processes().await;
            if procs[0].state() == ProcessState::Starting {
                return;
            }
            tokio::time::sleep(Duration::from_millis(5)).await;
        }
    })
    .await
    .expect("timed out waiting for in-flight spawn reservation");

    mgr.handle_stop("blocked-svc").await?;
    assert_eq!(mgr.processes().await[0].state(), ProcessState::Stopped);

    drop(_gate);
    spawn_task.await??;

    let procs = mgr.processes().await;
    assert_eq!(procs[0].state(), ProcessState::Stopped);
    assert!(!procs[0].is_running());
    assert!(procs[0].pid().is_none());

    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_concurrent_start_rejected_while_spawn_in_flight() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let _gate = super::super::spawn::close_spawn_gate_for_test();

    let mgr = ProcessManager::new(loader(vec![sleep_def("busy-svc")]), uuid_gen());
    let (handles, _rx) = test_runtime_context();

    let catalog = mgr.catalog.clone();
    let ctx = handles.clone();
    let spawn_task = tokio::spawn(async move {
        super::super::spawn::spawn_process(catalog, 0, &ctx, super::super::spawn::SpawnKind::Manual)
            .await
    });

    tokio::time::timeout(Duration::from_secs(1), async {
        loop {
            if mgr.processes().await[0].state() == ProcessState::Starting {
                return;
            }
            tokio::time::sleep(Duration::from_millis(5)).await;
        }
    })
    .await
    .expect("timed out waiting for in-flight spawn reservation");

    let err = mgr
        .handle_start("busy-svc", &handles)
        .await
        .expect_err("second start should fail while spawn is in flight");
    assert_eq!(err.code(), tonic::Code::FailedPrecondition);

    drop(_gate);
    spawn_task.await??;
    wait_until_running(&mgr, "busy-svc").await;

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_concurrent_start_only_one_succeeds() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();

    let mgr = ProcessManager::new(loader(vec![sleep_def("race-svc")]), uuid_gen());
    let (handles, _rx) = test_runtime_context();

    let mgr2 = mgr.clone();
    let handles2 = handles.clone();
    let (first, second) = tokio::join!(
        mgr.handle_start("race-svc", &handles),
        mgr2.handle_start("race-svc", &handles2),
    );

    let success_count = usize::from(first.is_ok()) + usize::from(second.is_ok());
    assert_eq!(
        success_count, 1,
        "exactly one concurrent start should succeed"
    );

    let err = if first.is_ok() {
        second.expect_err("second concurrent start should fail")
    } else {
        first.expect_err("first concurrent start should fail when second wins")
    };
    assert_eq!(err.code(), tonic::Code::FailedPrecondition);

    wait_until_running(&mgr, "race-svc").await;
    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_start_loses_spawn_reservation_returns_failed_precondition() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let _gate = super::super::spawn::close_spawn_gate_for_test();

    let mgr = ProcessManager::new(loader(vec![sleep_def("reserved-svc")]), uuid_gen());
    let (handles, _rx) = test_runtime_context();

    let catalog = mgr.catalog.clone();
    let ctx = handles.clone();
    let spawn_task = tokio::spawn(async move {
        super::super::spawn::spawn_process(catalog, 0, &ctx, super::super::spawn::SpawnKind::Manual)
            .await
    });

    tokio::time::timeout(Duration::from_secs(1), async {
        loop {
            if mgr.processes().await[0].state() == ProcessState::Starting {
                return;
            }
            tokio::time::sleep(Duration::from_millis(5)).await;
        }
    })
    .await
    .expect("timed out waiting for in-flight spawn reservation");

    let outcome = super::super::spawn::spawn_process(
        mgr.catalog.clone(),
        0,
        &handles,
        super::super::spawn::SpawnKind::Manual,
    )
    .await?;
    assert_eq!(
        outcome,
        super::super::spawn::SpawnProcessOutcome::NotStarted
    );

    drop(_gate);
    spawn_task.await??;
    wait_until_running(&mgr, "reserved-svc").await;

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_start_returns_committed_snapshot_after_immediate_exit() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();

    let mgr = ProcessManager::new(loader(vec![super::true_def("fast-exit")]), uuid_gen());
    let (ctx, mut rx) = test_runtime_context();

    let mgr_loop = mgr.clone();
    let ctx_loop = ctx.clone();
    let loop_task = tokio::spawn(async move {
        let pending = std::future::pending::<()>();
        tokio::pin!(pending);
        rx.run_with(&mgr_loop, &ctx_loop, pending).await;
    });

    let result = mgr.handle_start("fast-exit", &ctx).await?;
    assert_eq!(result.state, ProcessState::Running);
    assert!(result.pid.is_some());

    tokio::time::sleep(Duration::from_millis(100)).await;
    assert_ne!(
        mgr.processes().await[0].state(),
        ProcessState::Running,
        "exit watcher should update state after Start returns"
    );

    loop_task.abort();
    let _ = loop_task.await;
    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_create_auto_start_respects_in_flight_reservation() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (handles, _rx) = test_runtime_context();
    let (cmd, args) = test_helpers::sleep_cmd(60);

    mgr.handle_create(
        "auto-svc".to_string(),
        ProcessConfig {
            command: cmd.to_string(),
            args,
            auto_start: true,
            ..Default::default()
        },
        &handles,
    )
    .await?;

    tokio::time::timeout(Duration::from_secs(5), async {
        loop {
            let procs = mgr.processes().await;
            if procs[0].state() == ProcessState::Running {
                return;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    })
    .await
    .expect("auto-start should reach running");

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    Ok(())
}
