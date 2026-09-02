// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::super::*;
use super::{auto_start_for_test, current_pending_restart, loader, test_runtime_context, uuid_gen};
use crate::config::{ProcessConfig, ProcessDefinition, RestartPolicy};
use crate::config_gate::{ConditionConfigFile, test_env};
use crate::test_helpers;
use std::io::Write;

fn gated_sleep_def(name: &str, agent_yaml: &str) -> ProcessDefinition {
    let (cmd, args) = test_helpers::long_sleep_cmd();
    ProcessDefinition {
        name: name.to_string(),
        config: ProcessConfig {
            command: cmd.to_string(),
            args,
            condition_config_any: vec![ConditionConfigFile {
                path: agent_yaml.to_string(),
                keys: vec!["process_config.process_collection.enabled".into()],
            }],
            ..Default::default()
        },
    }
}

fn write_agent_yaml(dir: &std::path::Path, process_collection_enabled: bool) -> String {
    let path = dir.join("datadog.yaml");
    let body = format!(
        "process_config:\n  process_collection:\n    enabled: {process_collection_enabled}\n  container_collection:\n    enabled: false\n  process_discovery:\n    enabled: false\n"
    );
    let mut file = std::fs::File::create(&path).unwrap();
    file.write_all(body.as_bytes()).unwrap();
    path.to_string_lossy().into_owned()
}

fn gated_on_failure_sleep_def(name: &str, agent_yaml: &str) -> ProcessDefinition {
    let (cmd, args) = test_helpers::long_sleep_cmd();
    ProcessDefinition {
        name: name.to_string(),
        config: ProcessConfig {
            command: cmd.to_string(),
            args,
            restart: RestartPolicy::OnFailure,
            restart_sec: Some(2.0),
            condition_config_any: vec![ConditionConfigFile {
                path: agent_yaml.to_string(),
                keys: vec!["process_config.process_collection.enabled".into()],
            }],
            ..Default::default()
        },
    }
}

#[tokio::test]
async fn test_auto_start_runs_when_config_gate_open() -> anyhow::Result<()> {
    test_env::with_async_lock(|| async {
        let dir = tempfile::tempdir().unwrap();
        let yaml = write_agent_yaml(dir.path(), true);
        let mgr = ProcessManager::new(
            loader(vec![gated_sleep_def("gated-svc", &yaml)]),
            uuid_gen(),
        );
        let (handles, _rx) = test_runtime_context();

        auto_start_for_test(&mgr, &handles).await;
        assert!(
            mgr.processes().await[0].is_running(),
            "process should auto-start when condition_config_any is met"
        );

        mgr.shutdown().await;
    })
    .await;
    Ok(())
}

#[tokio::test]
async fn test_auto_start_skips_when_config_gate_closed() -> anyhow::Result<()> {
    test_env::with_async_lock(|| async {
        let dir = tempfile::tempdir().unwrap();
        let yaml = write_agent_yaml(dir.path(), false);
        let mgr = ProcessManager::new(
            loader(vec![gated_sleep_def("gated-svc", &yaml)]),
            uuid_gen(),
        );
        let (handles, _rx) = test_runtime_context();

        auto_start_for_test(&mgr, &handles).await;
        assert!(
            !mgr.processes().await[0].is_running(),
            "process should not auto-start when condition_config_any is not met"
        );
    })
    .await;
    Ok(())
}

#[tokio::test]
async fn test_on_failure_restart_skips_when_config_gate_closes() -> anyhow::Result<()> {
    test_env::with_async_lock(|| async {
        let dir = tempfile::tempdir().unwrap();
        let yaml = write_agent_yaml(dir.path(), true);
        let mgr = ProcessManager::new(
            loader(vec![gated_on_failure_sleep_def("gated-svc", &yaml)]),
            uuid_gen(),
        );
        let (handles, _rx) = test_runtime_context();

        auto_start_for_test(&mgr, &handles).await;
        let pid = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            procs[0].pid().unwrap()
        };

        write_agent_yaml(dir.path(), false);
        {
            let mut procs = mgr.catalog.write_processes().await;
            let (cmd, args) = test_helpers::false_cmd();
            let status = std::process::Command::new(cmd).args(args).status()?;
            procs[0].set_last_status(status);
        }
        test_helpers::cleanup_process(pid);

        let pending = {
            let procs = mgr.processes().await;
            current_pending_restart(&procs[0])
        };
        mgr.complete_restart(pending, &handles).await;
        assert!(
            !mgr.processes().await[0].is_running(),
            "on-failure restart should skip when the config gate is closed"
        );
        Ok::<(), anyhow::Error>(())
    })
    .await?;
    Ok(())
}
