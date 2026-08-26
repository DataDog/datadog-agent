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
use log::warn;
use std::sync::Arc;
use tokio::sync::RwLock;
use tonic::Status;

pub use process_manager::ProcessManager;
pub use supervisor::Supervisor;

#[cfg(all(test, unix))]
pub(crate) use supervisor::spawn_command_loop_for_tests;

pub(crate) type ExitEvent = crate::process::ProcessExit;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PendingRestart {
    pub(crate) uuid: String,
    pub(crate) spawn_seq: u64,
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

fn enqueue_pending_restart(proc: &mut ManagedProcess, handles: &RuntimeHandles) {
    if let Some(delay) = proc.schedule_restart() {
        let pending = PendingRestart {
            uuid: proc.uuid().to_owned(),
            spawn_seq: proc.spawn_seq(),
        };
        let tx = handles.restart_tx.clone();
        tokio::spawn(async move {
            tokio::time::sleep(delay).await;
            let _ = tx.send(pending).await;
        });
    }
}

fn try_auto_start(proc: &mut ManagedProcess, handles: &RuntimeHandles) {
    if !proc.may_auto_start() {
        return;
    }
    let name = proc.name().to_owned();
    if let Err(e) = proc.spawn_and_watch(handles.exit_tx.clone()) {
        warn!("[{name}] auto-start failed: {e:#}");
        enqueue_pending_restart(proc, handles);
    }
}

pub(in crate::manager) async fn run_auto_start_child_at(
    processes: Arc<RwLock<Vec<ManagedProcess>>>,
    idx: usize,
    handles: &RuntimeHandles,
) {
    let spawn_input = {
        let procs = processes.read().await;
        let Some(proc) = procs.get(idx) else {
            return;
        };
        if !proc.may_auto_start() {
            return;
        }
        Some((proc.name().to_owned(), proc.config().clone()))
    };
    let Some((name, config)) = spawn_input else {
        return;
    };

    let spawn_result = {
        #[cfg(windows)]
        let _console_guard = crate::platform::console_lock();
        crate::platform::spawn_managed_child(&name, &config)
    };

    let mut procs = processes.write().await;
    let Some(proc) = procs.get_mut(idx) else {
        if let Ok(outcome) = spawn_result {
            outcome.abort(&name).await;
        }
        return;
    };

    match spawn_result {
        Ok(outcome) => {
            if crate::platform::shutdown_requested() {
                outcome.abort(&name).await;
                return;
            }
            if let Err(e) = proc.spawn_and_watch_from_outcome(outcome, handles.exit_tx.clone()) {
                warn!("[{name}] auto-start failed: {e:#}");
                enqueue_pending_restart(proc, handles);
            }
        }
        Err(e) => {
            warn!("[{name}] auto-start failed: {e:#}");
            proc.mark_spawn_failed();
            if !crate::platform::shutdown_requested() {
                enqueue_pending_restart(proc, handles);
            }
        }
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
    use std::time::Duration;
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

    async fn auto_start_for_test(mgr: &ProcessManager, handles: &RuntimeHandles) {
        let _guard = crate::platform::test_shutdown_lock();
        crate::platform::reset_shutdown_state_for_test();
        mgr.auto_start_all(handles).await;
    }

    fn current_pending_restart(proc: &ManagedProcess) -> PendingRestart {
        PendingRestart {
            uuid: proc.uuid().to_owned(),
            spawn_seq: proc.spawn_seq(),
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

        auto_start_for_test(&mgr, &handles).await;

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
    async fn test_auto_start_all_skips_remaining_children_after_shutdown() -> anyhow::Result<()> {
        let _guard = crate::platform::test_shutdown_lock();
        crate::platform::reset_shutdown_state_for_test();
        let mgr = ProcessManager::new(
            loader(vec![sleep_def("first-child"), sleep_def("second-child")]),
            uuid_gen(),
        );
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        let mgr_task = mgr.clone();
        let handles_task = handles.clone();
        let auto_start_task =
            tokio::spawn(async move { mgr_task.auto_start_all(&handles_task).await });

        tokio::time::timeout(Duration::from_secs(5), async {
            loop {
                let procs = mgr.processes().await;
                if procs
                    .iter()
                    .any(|p| p.name() == "first-child" && p.is_running())
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

        assert!(crate::platform::shutdown_requested());
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
        let _guard = crate::platform::test_shutdown_lock();
        crate::platform::reset_shutdown_state_for_test();
        let mgr = ProcessManager::new(
            loader(vec![sleep_def("first-child"), sleep_def("second-child")]),
            uuid_gen(),
        );
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        let mgr_task = mgr.clone();
        let handles_task = handles.clone();
        let auto_start_task =
            tokio::spawn(async move { mgr_task.auto_start_all(&handles_task).await });

        tokio::time::timeout(Duration::from_secs(5), async {
            loop {
                let procs = mgr.processes().await;
                if procs
                    .iter()
                    .any(|p| p.name() == "first-child" && p.is_running())
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

        let _write_guard = tokio::time::timeout(Duration::from_millis(100), mgr.processes.write())
            .await
            .expect("catalog write lock still held after auto-start shutdown join");

        crate::platform::reset_shutdown_state_for_test();
        Ok(())
    }

    #[tokio::test]
    async fn test_shutdown_requested_during_auto_start() {
        let _guard = crate::platform::test_shutdown_lock();
        crate::platform::reset_shutdown_state_for_test();
        let mgr = ProcessManager::new(
            loader(vec![sleep_def("first-child"), sleep_def("second-child")]),
            uuid_gen(),
        );
        let (handles, _exit_rx, _restart_rx) = test_runtime_handles();

        crate::platform::signal_shutdown_for_test();
        mgr.auto_start_all(&handles).await;

        assert!(crate::platform::shutdown_requested());
        crate::platform::reset_shutdown_state_for_test();
    }

    #[tokio::test]
    async fn test_event_loop_runs_when_shutdown_not_requested() {
        use supervisor::run_manager_event_loop;
        use tokio::sync::mpsc;

        let _guard = crate::platform::test_shutdown_lock();
        crate::platform::reset_shutdown_state_for_test();
        let mgr = ProcessManager::new(loader(vec![]), uuid_gen());
        let (handles, mut exit_rx, mut restart_rx) = test_runtime_handles();
        let (_cmd_tx, mut cmd_rx) = mpsc::channel(1);

        mgr.auto_start_all(&handles).await;
        assert!(!crate::platform::shutdown_requested());

        let shutdown = crate::platform::shutdown_signal();
        tokio::pin!(shutdown);
        tokio::spawn(async {
            tokio::time::sleep(Duration::from_millis(10)).await;
            crate::platform::signal_shutdown_for_test();
        });

        run_manager_event_loop(
            &mgr,
            &handles,
            &mut cmd_rx,
            &mut exit_rx,
            &mut restart_rx,
            shutdown,
        )
        .await;

        crate::platform::reset_shutdown_state_for_test();
    }

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
        let (handles, _exit_rx, mut restart_rx) = test_runtime_handles();

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
            while let Some(p) = restart_rx.recv().await {
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

        mgr.handle_start("aabbccdd-1", &handles)
            .await
            .expect("unambiguous prefix should resolve");

        let _: Result<_, _> = mgr.handle_stop("aabbccdd-1").await;
    }
}
