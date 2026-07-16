// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![allow(clippy::result_large_err)]

mod process_manager;
mod reload;
mod supervisor;

use supervisor::RuntimeHandles;

use crate::config::ProcessDefinition;
use crate::ordering;
use crate::process::ManagedProcess;
use anyhow::Result;
use log::{debug, warn};
use std::sync::Arc;
use tokio::sync::mpsc;
use tonic::Status;

pub use process_manager::ProcessManager;
pub use supervisor::Supervisor;

#[cfg(test)]
pub(crate) use supervisor::spawn_command_loop_for_tests;

pub(crate) struct ExitEvent {
    pub name: String,
    pub pid: u32,
    pub status: std::process::ExitStatus,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PendingRestart {
    pub(super) uuid: String,
    pub(super) config_generation: u64,
}

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
        let pending = PendingRestart {
            uuid: proc.uuid().to_owned(),
            config_generation: proc.config_generation(),
        };
        let tx = handles.restart_sender();
        tokio::spawn(async move {
            tokio::time::sleep(delay).await;
            let _ = tx.send(pending).await;
        });
    }
}

fn try_spawn_and_watch(proc: &mut ManagedProcess, handles: &RuntimeHandles) -> Result<()> {
    proc.spawn()?;
    spawn_watcher(proc, handles.exit_sender());
    Ok(())
}

fn spawn_watcher(proc: &mut ManagedProcess, tx: mpsc::Sender<ExitEvent>) {
    if let Some(mut proc_handle) = proc.take_handle() {
        let name = proc.name().to_owned();
        let pid = proc.pid().unwrap_or(0);
        let watcher_handle = tokio::spawn(async move {
            let status = match proc_handle.wait().await {
                Ok(status) => status,
                Err(e) => {
                    warn!("[{name}] wait error: {e}, killing process");
                    let _ = proc_handle.kill().await;
                    match proc_handle.wait().await {
                        Ok(s) => s,
                        Err(e2) => {
                            warn!("[{name}] failed to reap after kill: {e2}");
                            return None;
                        }
                    }
                }
            };
            let _ = tx.try_send(ExitEvent {
                name: name.clone(),
                pid,
                status,
            });
            Some(status)
        });
        proc.set_watcher_handle(watcher_handle);
    }
}

struct StartupOrderResult {
    order: Vec<usize>,
    warnings: Vec<String>,
}

fn recompute_startup_order(procs: &[ManagedProcess]) -> StartupOrderResult {
    let defs: Vec<ProcessDefinition> = procs
        .iter()
        .map(|p| ProcessDefinition {
            name: p.name().to_string(),
            config: p.config().clone(),
        })
        .collect();
    let result = ordering::resolve_order(&defs);
    if !result.skipped.is_empty() {
        warn!(
            "dependency cycle detected, skipping processes: {}",
            result.skipped.join(", ")
        );
    }
    let names: Vec<&str> = result.order.iter().map(|&i| procs[i].name()).collect();
    debug!("startup order: {}", names.join(" -> "));
    StartupOrderResult {
        order: result.order,
        warnings: result.warnings,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{ConfigLoader, MutableConfigLoader, ProcessConfig, RestartPolicy, StaticConfigLoader};
    use crate::state::ProcessState;
    use crate::test_helpers;
    use crate::uuid_gen::{SequentialUuidGenerator, UuidGenerator, V4UuidGenerator};

    fn loader(defs: Vec<ProcessDefinition>) -> Arc<dyn ConfigLoader> {
        Arc::new(StaticConfigLoader::new(defs))
    }

    fn uuid_gen() -> Arc<dyn UuidGenerator> {
        Arc::new(V4UuidGenerator)
    }

    fn test_runtime_handles(
    ) -> (
        RuntimeHandles,
        mpsc::Receiver<ExitEvent>,
        mpsc::Receiver<PendingRestart>,
    ) {
        RuntimeHandles::new()
    }

    fn current_pending_restart(proc: &ManagedProcess) -> PendingRestart {
        PendingRestart {
            uuid: proc.uuid().to_owned(),
            config_generation: proc.config_generation(),
        }
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
        assert_eq!(
            pending.as_ref().map(|p| p.uuid.as_str()),
            Some(expected_uuid.as_str())
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_crash_retry_after_config_change() -> anyhow::Result<()> {
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
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

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
            tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
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

        mgr.complete_restart(stale_pending, &handles)
            .await;
        let pid = mgr.processes().await[0].pid().unwrap();
        test_helpers::cleanup_process(pid);
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_pending_retry_for_failed_auto_start_false() -> anyhow::Result<()>
    {
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
        let config_loader = Arc::new(MutableConfigLoader::new(vec![make_def(60)]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

        mgr.handle_start("action-executor", &handles).await?;
        let (pid, name) = {
            let procs = mgr.processes().await;
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
            tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
                .await
                .expect("timed out waiting for queued restart")
                .expect("expected queued restart event");

        config_loader.set(vec![make_def(90)]);
        mgr.handle_reload_config(&handles).await?;
        assert!(!mgr.processes().await[0].is_running());

        mgr.complete_restart(stale_pending, &handles)
            .await;
        assert!(
            !mgr.processes().await[0].is_running(),
            "config reload should discard pending crash retries for failed auto_start=false processes"
        );

        mgr.handle_start("action-executor", &handles).await?;
        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
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
        let config_loader = Arc::new(MutableConfigLoader::new(vec![make_def(60)]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
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
            procs[0].set_command_for_test("/nonexistent/dd-procmgr-failed-respawn".to_string());
        }

        mgr.complete_restart(pending, &handles).await;

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for second queued restart")
            .expect("expected second queued restart event");

        {
            let mut procs = mgr.processes.write().await;
            procs[0].set_command_for_test(cmd.to_string());
        }

        mgr.complete_restart(pending, &handles).await;
        assert!(mgr.processes().await[0].is_running());

        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_stale_bootstrap_retry_when_auto_start_disabled()
    -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-stale-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: true,
                ..Default::default()
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

        mgr.start_configured_processes(&handles).await;
        assert_eq!(mgr.processes().await[0].state(), ProcessState::Failed);

        config_loader.set(vec![ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-stale-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: false,
                ..Default::default()
            },
        }]);
        mgr.handle_reload_config(&handles).await?;

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for stale bootstrap retry")
            .expect("expected stale bootstrap retry event");
        mgr.complete_restart(pending, &handles).await;

        let procs = mgr.processes().await;
        assert!(!procs[0].is_running());
        assert_eq!(procs[0].state(), ProcessState::Failed);
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_discards_orphaned_retry_after_remove_and_readd() -> anyhow::Result<()> {
        let bad_def = ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-orphan-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: true,
                ..Default::default()
            },
        };
        let auto_start_false_def = ProcessDefinition {
            name: "svc".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-orphan-retry".to_string(),
                restart: RestartPolicy::OnFailure,
                restart_sec: Some(0.2),
                auto_start: false,
                ..Default::default()
            },
        };
        let config_loader = Arc::new(MutableConfigLoader::new(vec![bad_def]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

        mgr.start_configured_processes(&handles).await;
        let old_uuid = mgr.processes().await[0].uuid().to_owned();

        config_loader.set(vec![]);
        mgr.handle_reload_config(&handles).await?;

        config_loader.set(vec![auto_start_false_def]);
        mgr.handle_reload_config(&handles).await?;
        assert_ne!(mgr.processes().await[0].uuid(), old_uuid);

        let pending = tokio::time::timeout(std::time::Duration::from_secs(1), restart_rx.recv())
            .await
            .expect("timed out waiting for orphaned retry")
            .expect("expected orphaned retry event");
        assert_eq!(pending.uuid, old_uuid);

        mgr.complete_restart(pending, &handles).await;

        let procs = mgr.processes().await;
        assert!(!procs[0].is_running());
        assert_eq!(procs[0].state(), ProcessState::Created);
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
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.0),
                ..Default::default()
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
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
    async fn test_complete_restart_skips_stale_retry_when_restart_policy_revoked()
    -> anyhow::Result<()> {
        let (cmd, args) = test_helpers::sleep_cmd(60);
        let always_def = ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                restart: RestartPolicy::Always,
                restart_sec: Some(0.3),
                ..Default::default()
            },
        };
        let never_def = ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                command: test_helpers::sleep_cmd(60).0.to_string(),
                args: test_helpers::sleep_cmd(60).1,
                auto_start: false,
                restart: RestartPolicy::Never,
                restart_sec: Some(0.3),
                ..Default::default()
            },
        };
        let config_loader = Arc::new(MutableConfigLoader::new(vec![always_def]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
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

        config_loader.set(vec![never_def]);
        mgr.handle_reload_config(&handles).await?;
        assert_ne!(
            mgr.processes().await[0].config_generation(),
            pending.config_generation
        );

        mgr.complete_restart(pending, &handles).await;
        assert!(
            !mgr.processes().await[0].is_running(),
            "stale crash retry must not respawn after restart policy is revoked"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_updates_modified_config() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("svc-a", &handles).await?;
        let old_pid = {
            let procs = mgr.processes().await;
            assert!(procs[0].is_running());
            let expected_args = sleep_def("_").config.args;
            assert_eq!(procs[0].config().args, expected_args);
            procs[0].pid().unwrap()
        };

        // Reload with modified config (different args)
        config_loader.set(vec![sleep_def_secs("svc-a", 120)]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.modified.contains(&"svc-a".to_string()));
        assert!(result.added.is_empty());
        assert!(result.removed.is_empty());
        assert!(result.unchanged.is_empty());

        // Config should be updated and process restarted with a new PID
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
    async fn test_reload_modified_not_running_stays_stopped() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        // Don't start svc-a — leave it in Created state
        config_loader.set(vec![sleep_def_secs("svc-a", 120)]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.modified.contains(&"svc-a".to_string()));

        let procs = mgr.processes().await;
        let expected_args = sleep_def_secs("_", 120).config.args;
        assert_eq!(procs[0].config().args, expected_args);
        assert!(
            !procs[0].is_running(),
            "non-running modified process should not be started"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_retries_failed_auto_start_after_config_change() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                command: "/nonexistent/dd-procmgr-reload-retry".to_string(),
                restart: RestartPolicy::Never,
                ..Default::default()
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.start_configured_processes(&handles).await;
        assert_eq!(mgr.processes().await[0].state(), ProcessState::Failed);

        config_loader.set(vec![sleep_def("svc-a")]);
        mgr.handle_reload_config(&handles).await?;

        let procs = mgr.processes().await;
        assert!(
            procs[0].is_running(),
            "failed config-managed auto-start process should retry after definition change"
        );
        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_stopped_process_stays_stopped() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("svc-a", &handles).await?;
        mgr.handle_stop("svc-a").await?;
        assert_eq!(mgr.processes().await[0].state(), ProcessState::Stopped);

        config_loader.set(vec![sleep_def_secs("svc-a", 120)]);
        mgr.handle_reload_config(&handles).await?;

        let procs = mgr.processes().await;
        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert!(
            !procs[0].is_running(),
            "intentionally stopped process should not auto-retry after config change"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_unchanged_config_not_modified() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        // Reload with the exact same config
        config_loader.set(vec![sleep_def("svc-a")]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.unchanged.contains(&"svc-a".to_string()));
        assert!(result.modified.is_empty());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_restarts_running_auto_start_false_after_config_change()
    -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("action-executor", &handles).await?;
        let old_pid = mgr.processes().await[0].pid().unwrap();

        config_loader.set(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def_secs("action-executor", 120).config
            },
        }]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.modified.contains(&"action-executor".to_string()));

        let procs = mgr.processes().await;
        let expected_args = sleep_def_secs("_", 120).config.args;
        assert_eq!(procs[0].config().args, expected_args);
        assert!(
            procs[0].is_running(),
            "running auto_start=false process should restart after config change"
        );
        assert_ne!(procs[0].pid().unwrap(), old_pid);

        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_modified_auto_start_false_stopped_process_stays_stopped()
    -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("action-executor", &handles).await?;
        mgr.handle_stop("action-executor").await?;

        config_loader.set(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def_secs("action-executor", 120).config
            },
        }]);
        mgr.handle_reload_config(&handles).await?;

        let procs = mgr.processes().await;
        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert!(
            !procs[0].is_running(),
            "stopped auto_start=false process should not restart after config change"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_keeps_manually_started_auto_start_false_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("action-executor", &handles).await?;
        assert!(mgr.processes().await[0].is_running());

        config_loader.set(vec![ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                ..sleep_def("action-executor").config
            },
        }]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.unchanged.contains(&"action-executor".to_string()));

        let procs = mgr.processes().await;
        assert!(
            procs[0].is_running(),
            "manually started auto_start=false process should survive unchanged reload"
        );
        test_helpers::cleanup_process(procs[0].pid().unwrap());
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_stops_manually_started_auto_start_false_when_path_gate_closes()
    -> anyhow::Result<()> {
        let dir = tempfile::tempdir()?;
        let marker = dir.path().join("ready");
        std::fs::write(&marker, b"")?;
        let path_str = marker.to_str().unwrap().to_string();

        let def_with_path = |path: &str| ProcessDefinition {
            name: "action-executor".to_string(),
            config: ProcessConfig {
                auto_start: false,
                condition_path_exists: Some(path.to_string()),
                ..sleep_def("action-executor").config
            },
        };

        let config_loader = Arc::new(MutableConfigLoader::new(vec![def_with_path(&path_str)]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("action-executor", &handles).await?;
        assert!(mgr.processes().await[0].is_running());

        std::fs::remove_file(&marker)?;
        config_loader.set(vec![def_with_path(&path_str)]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.unchanged.contains(&"action-executor".to_string()));
        assert!(
            !mgr.processes().await[0].is_running(),
            "manually started auto_start=false process should stop when path gate closes"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_reload_restarts_process_when_path_reappears() -> anyhow::Result<()> {
        let dir = tempfile::tempdir()?;
        let marker = dir.path().join("ready");
        std::fs::write(&marker, b"")?;
        let path_str = marker.to_str().unwrap().to_string();

        let def_with_path = |path: &str| ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                auto_start: true,
                condition_path_exists: Some(path.to_string()),
                ..sleep_def("svc-a").config
            },
        };

        let config_loader = Arc::new(MutableConfigLoader::new(vec![]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        config_loader.set(vec![def_with_path(&path_str)]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.added.contains(&"svc-a".to_string()));
        assert!(mgr.processes().await[0].is_running());

        std::fs::remove_file(&marker)?;
        config_loader.set(vec![def_with_path(&path_str)]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.unchanged.contains(&"svc-a".to_string()));
        assert!(
            !mgr.processes().await[0].is_running(),
            "process should stop when condition_path_exists is no longer met"
        );

        std::fs::write(&marker, b"")?;
        config_loader.set(vec![def_with_path(&path_str)]);
        mgr.handle_reload_config(&handles).await?;
        assert!(
            mgr.processes().await[0].is_running(),
            "process should restart when condition_path_exists is met again"
        );

        test_helpers::cleanup_process(mgr.processes().await[0].pid().unwrap());
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
    async fn test_reload_preserves_runtime_created_processes() -> anyhow::Result<()> {
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (cmd, args) = test_helpers::true_cmd();
        let config = ProcessConfig {
            command: cmd.to_string(),
            args,
            ..Default::default()
        };
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();
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
    async fn test_shutdown_after_reload_removes_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![
            sleep_def("svc-a"),
            sleep_def("svc-b"),
        ]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("svc-a", &handles).await?;
        mgr.handle_start("svc-b", &handles).await?;

        // Reload removes svc-b
        config_loader.set(vec![sleep_def("svc-a")]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.removed.contains(&"svc-b".to_string()));

        // Shutdown must not panic despite svc-b being gone from the Vec
        mgr.shutdown().await;

        let procs = mgr.processes().await;
        assert!(
            procs.iter().all(|p| !p.is_running()),
            "all remaining processes should be stopped"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_shutdown_after_reload_adds_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("svc-a", &handles).await?;

        // Reload adds svc-b
        config_loader.set(vec![sleep_def("svc-a"), sleep_def("svc-b")]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.added.contains(&"svc-b".to_string()));

        // svc-b auto-started by reload; start svc-a again is already running
        mgr.shutdown().await;

        let procs = mgr.processes().await;
        assert!(
            procs.iter().all(|p| !p.is_running()),
            "all processes (including reload-added) should be stopped"
        );
        Ok(())
    }

    #[tokio::test]
    async fn test_shutdown_after_reload_with_runtime_process() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        mgr.handle_start("svc-a", &handles).await?;

        // Create a runtime process
        let (cmd, args) = test_helpers::sleep_cmd(60);
        mgr.handle_create(
            "runtime-svc".to_string(),
            ProcessConfig {
                command: cmd.to_string(),
                args,
                auto_start: false,
                ..Default::default()
            },
            &handles,
        )
        .await?;
        mgr.handle_start("runtime-svc", &handles).await?;

        // Reload removes svc-a but preserves runtime-svc
        config_loader.set(vec![]);
        let result = mgr.handle_reload_config(&handles).await?;
        assert!(result.removed.contains(&"svc-a".to_string()));

        mgr.shutdown().await;

        let procs = mgr.processes().await;
        assert!(
            procs.iter().all(|p| !p.is_running()),
            "runtime-created process should also be shut down"
        );
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
    async fn test_reload_recomputes_startup_order() -> anyhow::Result<()> {
        let config_loader = Arc::new(MutableConfigLoader::new(vec![sleep_def("svc-a")]));
        let mgr = ProcessManager::new(config_loader.clone(), uuid_gen());
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        {
            let order = mgr.startup_order.read().await;
            assert_eq!(*order, vec![0], "single process at index 0");
        }

        // Reload with a new process that has an after-dependency, which
        // forces a non-alphabetical order (svc-b before svc-api).
        let (cmd, args) = test_helpers::sleep_cmd(60);
        config_loader.set(vec![
            ProcessDefinition {
                name: "svc-api".to_string(),
                config: ProcessConfig {
                    command: cmd.to_string(),
                    args,
                    after: vec!["svc-b".to_string()],
                    ..Default::default()
                },
            },
            sleep_def("svc-b"),
        ]);
        mgr.handle_reload_config(&handles).await?;

        let order = mgr.startup_order.read().await;
        let procs = mgr.processes().await;
        let names: Vec<&str> = order.iter().map(|&i| procs[i].name()).collect();
        assert_eq!(
            names,
            vec!["svc-b", "svc-api"],
            "startup order should be recomputed with dependency constraints"
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
