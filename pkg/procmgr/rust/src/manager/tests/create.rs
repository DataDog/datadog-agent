// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::super::*;
use super::{loader, sleep_def, test_runtime_context, uuid_gen, wait_until_running};
use crate::config::ProcessConfig;
use crate::state::ProcessState;
use crate::test_helpers;

#[cfg(unix)]
#[tokio::test]
async fn test_create_auto_start_reserves_before_return() -> anyhow::Result<()> {
    let _guard = super::test_manager_lock().await;
    crate::platform::reset_shutdown_state_for_test();
    let _gate = super::super::spawn::close_spawn_gate_for_test();

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

    assert_eq!(mgr.processes().await[0].state(), ProcessState::Starting);
    mgr.handle_stop("auto-svc").await?;

    drop(_gate);
    handles.background_spawns.join_all().await;

    let proc = &mgr.processes().await[0];
    assert_eq!(proc.state(), ProcessState::Stopped);
    assert!(!proc.is_running());
    assert!(proc.pid().is_none());

    crate::platform::reset_shutdown_state_for_test();
    Ok(())
}

#[tokio::test]
async fn test_create_rejects_empty_name() {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (cmd, args) = test_helpers::true_cmd();
    let config = ProcessConfig {
        command: cmd.to_string(),
        args,
        ..Default::default()
    };
    let (handles, _rx) = test_runtime_context();
    let err = mgr
        .handle_create("".to_string(), config, &handles)
        .await
        .unwrap_err();
    assert_eq!(err.code(), tonic::Code::InvalidArgument);
}

#[tokio::test]
async fn test_create_rejects_invalid_name() {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (cmd, args) = test_helpers::true_cmd();
    let config = ProcessConfig {
        command: cmd.to_string(),
        args,
        ..Default::default()
    };
    let (handles, _rx) = test_runtime_context();
    let err = mgr
        .handle_create("bad name!".to_string(), config, &handles)
        .await
        .unwrap_err();
    assert_eq!(err.code(), tonic::Code::InvalidArgument);
}

#[tokio::test]
async fn test_create_accepts_valid_name() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (cmd, args) = test_helpers::true_cmd();
    let config = ProcessConfig {
        command: cmd.to_string(),
        args,
        ..Default::default()
    };
    let (handles, _rx) = test_runtime_context();
    mgr.handle_create("my-svc_v2.0".to_string(), config, &handles)
        .await?;
    let procs = mgr.processes().await;
    assert_eq!(procs[0].name(), "my-svc_v2.0");
    Ok(())
}

#[tokio::test]
async fn test_startup_order_indices_match_processes() {
    let mgr = ProcessManager::new(
        loader(vec![
            sleep_def("alpha"),
            sleep_def("bravo"),
            sleep_def("charlie"),
        ]),
        uuid_gen(),
    );

    let order = mgr.catalog.startup_order().await;
    let procs = mgr.processes().await;
    let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
    assert_eq!(names, vec!["alpha", "bravo", "charlie"]);
}

#[tokio::test]
async fn test_create_includes_runtime_process_in_startup_order() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![sleep_def("svc-a")]), uuid_gen());
    let (handles, _rx) = test_runtime_context();
    let (cmd, args) = test_helpers::sleep_cmd(60);
    mgr.handle_create(
        "svc-b".to_string(),
        ProcessConfig {
            command: cmd.to_string(),
            args,
            after: vec!["svc-a".to_string()],
            auto_start: false,
            ..Default::default()
        },
        &handles,
    )
    .await?;

    let order = mgr.catalog.startup_order().await;
    let procs = mgr.processes().await;
    let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
    assert_eq!(
        names,
        vec!["svc-a", "svc-b"],
        "runtime process with after-dep should appear in startup order"
    );
    Ok(())
}

#[tokio::test]
async fn test_create_auto_start_spawns_process() -> anyhow::Result<()> {
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

    wait_until_running(&mgr, "auto-svc").await;
    handles.background_spawns.join_all().await;
    {
        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert!(
            procs[0].is_running(),
            "process with auto_start=true should be running after create"
        );
        assert!(
            procs[0].pid().is_some(),
            "running process should have a PID"
        );
    }

    mgr.shutdown().await;
    Ok(())
}

#[tokio::test]
async fn test_create_auto_start_false_stays_created() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (handles, _rx) = test_runtime_context();
    let (cmd, args) = test_helpers::sleep_cmd(60);
    mgr.handle_create(
        "manual-svc".to_string(),
        ProcessConfig {
            command: cmd.to_string(),
            args,
            auto_start: false,
            ..Default::default()
        },
        &handles,
    )
    .await?;

    let procs = mgr.processes().await;
    assert_eq!(procs.len(), 1);
    assert!(
        !procs[0].is_running(),
        "process with auto_start=false should not be running after create"
    );
    Ok(())
}

#[tokio::test]
async fn test_create_auto_start_bad_command_still_created() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (handles, _rx) = test_runtime_context();
    let result = mgr
        .handle_create(
            "bad-cmd".to_string(),
            ProcessConfig {
                command: "/nonexistent/binary".to_string(),
                auto_start: true,
                ..Default::default()
            },
            &handles,
        )
        .await;

    assert!(result.is_ok(), "create should succeed even if spawn fails");
    handles.background_spawns.join_all().await;
    let procs = mgr.processes().await;
    assert_eq!(procs.len(), 1);
    assert_eq!(procs[0].name(), "bad-cmd");
    assert_eq!(procs[0].state(), ProcessState::Failed);
    Ok(())
}

#[tokio::test]
async fn test_create_auto_start_condition_not_met() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (handles, _rx) = test_runtime_context();
    let (cmd, args) = test_helpers::sleep_cmd(60);
    mgr.handle_create(
        "cond-svc".to_string(),
        ProcessConfig {
            command: cmd.to_string(),
            args,
            auto_start: true,
            condition_path_exists: Some("/nonexistent/path/that/should/not/exist".to_string()),
            ..Default::default()
        },
        &handles,
    )
    .await?;

    let procs = mgr.processes().await;
    assert_eq!(procs.len(), 1);
    assert!(
        !procs[0].is_running(),
        "process should not start when condition_path_exists is not met"
    );
    Ok(())
}

#[tokio::test]
async fn test_concurrent_create_preserves_startup_order() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (handles, _rx) = test_runtime_context();
    const CREATE_COUNT: usize = 16;
    let mut tasks = Vec::with_capacity(CREATE_COUNT);

    for i in 0..CREATE_COUNT {
        let mgr = mgr.clone();
        let handles = handles.clone();
        let (cmd, args) = test_helpers::true_cmd();
        tasks.push(tokio::spawn(async move {
            mgr.handle_create(
                format!("svc-{i}"),
                ProcessConfig {
                    command: cmd.to_string(),
                    args,
                    auto_start: false,
                    ..Default::default()
                },
                &handles,
            )
            .await
        }));
    }

    for task in tasks {
        task.await??;
    }

    let order = mgr.catalog.startup_order().await;
    let procs = mgr.processes().await;
    assert_eq!(procs.len(), CREATE_COUNT);
    assert_eq!(order.len(), CREATE_COUNT);

    let mut seen = std::collections::HashSet::new();
    for &idx in order.iter() {
        assert!(idx < procs.len());
        assert!(seen.insert(idx), "duplicate startup index {idx}");
    }
    Ok(())
}
