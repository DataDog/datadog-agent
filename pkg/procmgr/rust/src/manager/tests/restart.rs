// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::super::*;
use super::{
    current_pending_restart, loader, sleep_def, test_runtime_context, uuid_gen, wait_until_running,
};
use crate::config::{ProcessConfig, ProcessDefinition, RestartPolicy};
use crate::test_helpers;

#[tokio::test]
async fn test_enqueue_pending_restart_retries_after_failed_respawn() -> anyhow::Result<()> {
    let (cmd, _args) = test_helpers::sleep_cmd(60);
    let make_def = |secs: u32| ProcessDefinition {
        name: "action-executor".to_string(),
        config: ProcessConfig {
            command: cmd.to_string(),
            args: test_helpers::sleep_cmd(secs).1,
            auto_start: false,
            restart: RestartPolicy::Always,
            restart_sec: Some(0.0),
            runtime_success_sec: Some(0),
            ..Default::default()
        },
    };
    let mgr = ProcessManager::new(loader(vec![make_def(60)]), uuid_gen());
    let (handles, mut rx) = test_runtime_context();

    mgr.handle_start("action-executor", &handles).await?;
    let (pid, name) = {
        let procs = mgr.processes().await;
        assert!(procs[0].is_running());
        (procs[0].pid().unwrap(), procs[0].name().to_owned())
    };

    tokio::time::sleep(std::time::Duration::from_millis(50)).await;
    test_helpers::cleanup_process(pid);
    mgr.handle_exit(
        ExitEvent {
            name,
            pid,
            status: test_helpers::exit_status(1),
        },
        &handles,
    )
    .await;

    let pending = tokio::time::timeout(std::time::Duration::from_secs(1), rx.restart_rx.recv())
        .await
        .expect("timed out waiting for first queued restart")
        .expect("expected first queued restart event");

    {
        let mut procs = mgr.processes.write().await;
        procs[0].config_mut().command = "/nonexistent/dd-procmgr-failed-respawn".to_string();
    }

    mgr.complete_restart(pending, &handles).await;

    let pending = tokio::time::timeout(std::time::Duration::from_secs(1), rx.restart_rx.recv())
        .await
        .expect("timed out waiting for second queued restart")
        .expect("expected second queued restart event");

    {
        let mut procs = mgr.processes.write().await;
        procs[0].config_mut().command = cmd.to_string();
    }

    mgr.complete_restart(pending, &handles).await;
    wait_until_running(&mgr, "action-executor").await;
    assert!(mgr.processes().await[0].is_running());

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    Ok(())
}

#[tokio::test]
async fn test_stale_restart_timer_invalidated_after_manual_start() -> anyhow::Result<()> {
    let (cmd, args) = test_helpers::sleep_cmd(60);
    let mgr = ProcessManager::new(
        loader(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(1.0),
                ..Default::default()
            },
        }]),
        uuid_gen(),
    );
    let (handles, mut rx) = test_runtime_context();

    mgr.handle_start("action-executor", &handles).await?;
    let (uuid, pid, name) = {
        let procs = mgr.processes().await;
        assert_eq!(procs[0].spawn_seq(), 1);
        (
            procs[0].uuid().to_owned(),
            procs[0].pid().unwrap(),
            procs[0].name().to_owned(),
        )
    };

    test_helpers::cleanup_process(pid);
    mgr.handle_exit(
        ExitEvent {
            name,
            pid,
            status: test_helpers::exit_status(1),
        },
        &handles,
    )
    .await;

    let stale_pending = PendingRestart {
        uuid: uuid.clone(),
        spawn_seq: 1,
    };

    mgr.handle_start("action-executor", &handles).await?;
    let (pid, name) = {
        let procs = mgr.processes().await;
        assert_eq!(
            procs[0].spawn_seq(),
            2,
            "manual start should bump successful-spawn seq"
        );
        (procs[0].pid().unwrap(), procs[0].name().to_owned())
    };

    test_helpers::cleanup_process(pid);
    mgr.handle_exit(
        ExitEvent {
            name,
            pid,
            status: test_helpers::exit_status(1),
        },
        &handles,
    )
    .await;

    mgr.complete_restart(stale_pending, &handles).await;
    assert!(
        !mgr.processes().await[0].is_running(),
        "stale queued restart must not respawn after a newer manual start"
    );

    let pending = tokio::time::timeout(std::time::Duration::from_secs(3), async {
        while let Some(p) = rx.restart_rx.recv().await {
            if p.spawn_seq == 2 {
                return Some(p);
            }
        }
        None
    })
    .await
    .expect("timed out waiting for current spawn_seq restart")
    .expect("expected restart event for current spawn_seq");

    mgr.complete_restart(pending, &handles).await;
    wait_until_running(&mgr, "action-executor").await;
    assert!(mgr.processes().await[0].is_running());

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    Ok(())
}

#[tokio::test]
async fn test_complete_restart_skips_already_running() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![sleep_def("svc")]), uuid_gen());
    let (handles, _rx) = test_runtime_context();

    mgr.handle_start("svc", &handles).await?;
    let pending = {
        let procs = mgr.processes().await;
        assert!(procs[0].is_running());
        current_pending_restart(&procs[0])
    };
    mgr.complete_restart(pending, &handles).await;

    let procs = mgr.processes().await;
    assert_eq!(procs.len(), 1);
    assert!(procs[0].is_running());

    test_helpers::cleanup_process(procs[0].pid().unwrap());
    Ok(())
}

#[tokio::test]
async fn test_complete_restart_honors_policy_for_auto_start_false() -> anyhow::Result<()> {
    let (cmd, args) = test_helpers::sleep_cmd(60);
    let mgr = ProcessManager::new(
        loader(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.0),
                ..Default::default()
            },
        }]),
        uuid_gen(),
    );
    let (handles, _rx) = test_runtime_context();

    mgr.handle_start("action-executor", &handles).await?;
    {
        let procs = mgr.processes().await;
        assert!(procs[0].is_running());
        assert!(!procs[0].may_auto_start());
        assert!(procs[0].start_conditions_met());
    }

    {
        let mut procs = mgr.processes.write().await;
        let (cmd, args) = test_helpers::false_cmd();
        let status = std::process::Command::new(cmd).args(args).status()?;
        procs[0].set_last_status(status);
    }
    assert!(!mgr.processes().await[0].is_running());

    let pending = {
        let procs = mgr.processes().await;
        current_pending_restart(&procs[0])
    };
    mgr.complete_restart(pending, &handles).await;
    wait_until_running(&mgr, "action-executor").await;
    assert!(
        mgr.processes().await[0].is_running(),
        "auto_start=false process should still restart when restart policy allows"
    );

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    Ok(())
}

#[tokio::test]
async fn test_complete_restart_skips_retry_when_restart_policy_revoked() -> anyhow::Result<()> {
    let (cmd, args) = test_helpers::sleep_cmd(60);
    let mgr = ProcessManager::new(
        loader(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args: args.clone(),
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.3),
                ..Default::default()
            },
        }]),
        uuid_gen(),
    );
    let (handles, mut rx) = test_runtime_context();

    mgr.handle_start("action-executor", &handles).await?;
    let (pid, name) = {
        let procs = mgr.processes().await;
        assert!(procs[0].is_running());
        (procs[0].pid().unwrap(), procs[0].name().to_owned())
    };

    tokio::time::sleep(std::time::Duration::from_millis(50)).await;
    test_helpers::cleanup_process(pid);
    mgr.handle_exit(
        ExitEvent {
            name,
            pid,
            status: test_helpers::exit_status(1),
        },
        &handles,
    )
    .await;

    let pending = tokio::time::timeout(std::time::Duration::from_secs(1), rx.restart_rx.recv())
        .await
        .expect("timed out waiting for queued restart")
        .expect("expected queued restart event");

    {
        let mut procs = mgr.processes.write().await;
        procs[0].config_mut().restart = RestartPolicy::Never;
    }

    mgr.complete_restart(pending, &handles).await;
    assert!(
        !mgr.processes().await[0].is_running(),
        "queued restart must not respawn after restart policy is revoked"
    );
    Ok(())
}
