// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![allow(clippy::result_large_err)]

mod process_manager;
mod supervisor;

use supervisor::RuntimeHandles;

use crate::process::ManagedProcess;
use anyhow::Result;
use tonic::Status;

pub use process_manager::ProcessManager;
pub use supervisor::Supervisor;

#[cfg(all(test, unix))]
pub(crate) use supervisor::spawn_command_loop_for_tests;

pub(crate) type ExitEvent = crate::process::ProcessExit;

pub(crate) type PendingRestart = String;

pub fn looks_like_uuid_prefix(s: &str) -> bool {
    s.len() >= 8 && s.chars().all(|c| c.is_ascii_hexdigit() || c == '-')
}

fn resolve_by_uuid_prefix(procs: &[ManagedProcess], prefix: &str) -> Option<Result<usize, Status>> {
    let mut matches: Vec<usize> = procs
        .iter()
        .enumerate()
        .filter(|(_, p)| p.uuid().starts_with(prefix))
        .map(|(i, _)| i)
        .collect();
    match matches.len() {
        0 => None,
        1 => Some(Ok(matches.remove(0))),
        _ => Some(Err(Status::invalid_argument(format!(
            "UUID prefix '{prefix}' is ambiguous ({} matches)",
            matches.len()
        )))),
    }
}

fn find_index_by_name(procs: &[ManagedProcess], name: &str) -> Option<usize> {
    procs.iter().position(|p| p.name() == name)
}

fn resolve_index(procs: &[ManagedProcess], name_or_uuid: &str) -> Result<usize, Status> {
    if looks_like_uuid_prefix(name_or_uuid)
        && let Some(result) = resolve_by_uuid_prefix(procs, name_or_uuid)
    {
        return result;
    }
    find_index_by_name(procs, name_or_uuid)
        .ok_or_else(|| Status::not_found(format!("process '{name_or_uuid}' not found")))
}

fn queue_restart(proc: &mut ManagedProcess, handles: &RuntimeHandles) {
    if let Some(delay) = proc.schedule_restart() {
        let pending = proc.uuid().to_owned();
        let tx = handles.restart_tx.clone();
        tokio::spawn(async move {
            tokio::time::sleep(delay).await;
            let _ = tx.send(pending).await;
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{
        ConfigLoader, ProcessConfig, ProcessDefinition, RestartPolicy, StaticConfigLoader,
    };
    use crate::test_helpers;
    use crate::uuid_gen::{SequentialUuidGenerator, UuidGenerator, V4UuidGenerator};
    use std::sync::Arc;
    use tokio::sync::mpsc;

    fn loader(defs: Vec<ProcessDefinition>) -> Arc<dyn ConfigLoader> {
        Arc::new(StaticConfigLoader::new(defs))
    }

    fn uuid_gen() -> Arc<dyn UuidGenerator> {
        Arc::new(V4UuidGenerator)
    }

    fn test_runtime_handles() -> (
        RuntimeHandles,
        mpsc::Receiver<ExitEvent>,
        mpsc::Receiver<PendingRestart>,
    ) {
        RuntimeHandles::new()
    }

    fn current_pending_restart(proc: &ManagedProcess) -> PendingRestart {
        proc.uuid().to_owned()
    }

    fn sleep_def(name: &str) -> ProcessDefinition {
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
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

        mgr.start_configured_processes(&handles).await;

        assert!(!mgr.processes().await[0].is_running());
        let expected_uuid = mgr.processes().await[0].uuid().to_owned();
        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for restart after spawn failure");
        assert_eq!(pending.as_deref(), Some(expected_uuid.as_str()));
        Ok(())
    }

    #[tokio::test]
    async fn test_queue_restart_retries_after_failed_respawn() -> anyhow::Result<()> {
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
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

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

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for first queued restart")
            .expect("expected first queued restart event");

        {
            let mut procs = mgr.processes.write().await;
            procs[0].config_mut().command = "/nonexistent/dd-procmgr-failed-respawn".to_string();
        }

        mgr.complete_restart(pending, &handles).await;

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for second queued restart")
            .expect("expected second queued restart event");

        {
            let mut procs = mgr.processes.write().await;
            procs[0].config_mut().command = cmd.to_string();
        }

        mgr.complete_restart(pending, &handles).await;
        assert!(mgr.processes().await[0].is_running());

        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_complete_restart_skips_already_running() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![sleep_def("svc")]), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

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
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

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
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

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

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
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

    #[tokio::test]
    async fn test_create_rejects_empty_name() {
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (cmd, args) = test_helpers::true_cmd();
        let config = ProcessConfig {
            command: cmd.to_string(),
            args,
            ..Default::default()
        };
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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

        let order = mgr.startup_order.read().await;
        let procs = mgr.processes().await;
        let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
        assert_eq!(names, vec!["alpha", "bravo", "charlie"]);
    }

    #[tokio::test]
    async fn test_create_includes_runtime_process_in_startup_order() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![sleep_def("svc-a")]), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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

        let order = mgr.startup_order.read().await;
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
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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
        let procs = mgr.processes().await;
        assert_eq!(procs.len(), 1);
        assert_eq!(procs[0].name(), "bad-cmd");
        assert!(
            !procs[0].is_running(),
            "process with bad command should not be running"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_create_auto_start_condition_not_met() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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
    async fn test_ambiguous_uuid_prefix_returns_error() {
        // Both UUIDs share the first 8 characters ("aabbccdd"), which is the
        // length shown by `dd-procmgr list`.
        let uuid_gen: Arc<dyn UuidGenerator> = Arc::new(SequentialUuidGenerator::new(vec![
            "aabbccdd-1111-0000-0000-000000000000",
            "aabbccdd-2222-0000-0000-000000000000",
        ]));
        let mgr = ProcessManager::new(
            loader(vec![sleep_def("svc-a"), sleep_def("svc-b")]),
            uuid_gen,
        );
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        let err: Status = mgr.handle_start("aabbccdd", &handles).await.unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
        assert!(
            err.message().contains("ambiguous"),
            "error should mention ambiguity: {}",
            err.message()
        );

        // A longer, unambiguous prefix should resolve correctly.
        mgr.handle_start("aabbccdd-1", &handles)
            .await
            .expect("unambiguous prefix should resolve");

        // Clean up the spawned process.
        let _: Result<_, _> = mgr.handle_stop("aabbccdd-1").await;
    }
}
