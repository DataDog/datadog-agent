// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::super::*;
#[cfg(not(windows))]
use super::auto_start_for_test;
#[cfg(not(windows))]
use super::config_gate::{gated_sleep_def, write_agent_yaml};
use super::{loader, sleep_def, sleep_def_secs, test_runtime_context, uuid_gen};
use crate::config::{MutableConfigLoader, ProcessConfig, ProcessDefinition, RestartPolicy};
use crate::test_helpers;
use std::sync::Arc;

#[cfg(not(windows))]
#[tokio::test]
async fn test_reload_starts_when_config_gate_opens() -> anyhow::Result<()> {
    let dir = tempfile::tempdir()?;
    let yaml = write_agent_yaml(dir.path(), false);

    let config_loader = Arc::new(MutableConfigLoader::new(vec![gated_sleep_def(
        "svc-gated",
        &yaml,
    )]));
    let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
    let (handles, _rx) = test_runtime_context();
    auto_start_for_test(&mgr, &handles).await;
    assert!(
        !mgr.processes().await[0].is_running(),
        "gated process should not auto-start when collection is disabled"
    );

    write_agent_yaml(dir.path(), true);
    config_loader.set(vec![gated_sleep_def("svc-gated", &yaml)]);
    mgr.handle_reload_config(&handles).await?;
    assert!(
        mgr.processes().await[0].is_running(),
        "reload should start the process when the config gate opens"
    );

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    Ok(())
}

#[tokio::test]
async fn test_reload_updates_modified_config() -> anyhow::Result<()> {
    let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
    let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
    let (handles, _rx) = test_runtime_context();

    mgr.handle_start("svc-a", &handles).await?;
    let old_pid = {
        let procs = mgr.processes().await;
        assert!(procs[0].is_running());
        procs[0].pid().unwrap()
    };

    config_loader.set(vec![sleep_def_secs("svc-a", 120)]);
    let result = mgr.handle_reload_config(&handles).await?;
    assert!(result.modified.contains(&"svc-a".to_string()));
    assert!(result.added.is_empty());
    assert!(result.removed.is_empty());
    assert!(result.unchanged.is_empty());

    let procs = mgr.processes().await;
    let expected_args = sleep_def_secs("_", 120).config.args;
    assert_eq!(procs[0].config().args, expected_args);
    assert!(
        procs[0].is_running(),
        "modified running process should be restarted"
    );
    assert_ne!(
        procs[0].pid().unwrap(),
        old_pid,
        "restarted process should have a different PID"
    );

    test_helpers::cleanup_process(procs[0].pid().unwrap());
    Ok(())
}

#[tokio::test]
async fn test_reload_unchanged_config_not_modified() -> anyhow::Result<()> {
    let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
    let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
    let (handles, _rx) = test_runtime_context();

    mgr.handle_start("svc-a", &handles).await?;
    config_loader.set(vec![sleep_def("svc-a")]);
    let result = mgr.handle_reload_config(&handles).await?;
    assert!(result.unchanged.contains(&"svc-a".to_string()));
    assert!(result.modified.is_empty());
    assert!(mgr.processes().await[0].is_running());

    test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
    Ok(())
}

#[tokio::test]
async fn test_reload_preserves_runtime_created_processes() -> anyhow::Result<()> {
    let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
    let (cmd, args) = test_helpers::true_cmd();
    let config = ProcessConfig {
        command: cmd.to_string(),
        args,
        ..Default::default()
    };
    let (handles, _rx) = test_runtime_context();
    mgr.handle_create("runtime-svc".to_string(), config, &handles)
        .await?;

    let result = mgr.handle_reload_config(&handles).await?;
    assert!(
        !result.removed.contains(&"runtime-svc".to_string()),
        "runtime-created process should not be removed by reload"
    );

    let procs = mgr.processes().await;
    assert_eq!(procs.len(), 1);
    assert_eq!(procs[0].name(), "runtime-svc");
    Ok(())
}

#[tokio::test]
async fn test_reload_discards_stale_restart_after_config_change() -> anyhow::Result<()> {
    let (cmd, _) = test_helpers::sleep_cmd(60);
    let make_def = |secs: u32| ProcessDefinition {
        name: "svc".to_string(),
        config: ProcessConfig {
            command: cmd.to_string(),
            args: test_helpers::sleep_cmd(secs).1,
            auto_start: true,
            restart: RestartPolicy::Always,
            restart_sec: Some(0.3),
            runtime_success_sec: Some(0),
            ..Default::default()
        },
    };
    let config_loader = Arc::new(MutableConfigLoader::new(vec![make_def(60)]));
    let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
    let (handles, mut rx) = test_runtime_context();

    mgr.handle_start("svc", &handles).await?;
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

    let stale_pending =
        tokio::time::timeout(std::time::Duration::from_secs(1), rx.restart_rx.recv())
            .await
            .expect("timed out waiting for queued restart")
            .expect("expected queued restart event");

    config_loader.set(vec![make_def(120)]);
    mgr.handle_reload_config(&handles).await?;
    assert_ne!(
        mgr.processes().await[0].config_generation(),
        stale_pending.config_generation
    );
    assert!(
        mgr.processes().await[0].is_running(),
        "reload should start the process with fresh counters"
    );

    mgr.complete_restart(stale_pending, &handles).await;
    let pid = mgr.processes().await[0].pid().unwrap();
    test_helpers::cleanup_process(pid);
    Ok(())
}
