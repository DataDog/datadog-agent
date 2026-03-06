// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::ProcessManager;
use crate::grpc::proto;
use crate::process::ManagedProcess;
use crate::state::ProcessState;
use std::time::Instant;
use tonic::{Request, Response, Status};

pub struct ProcessManagerService {
    mgr: ProcessManager,
    started_at: Instant,
    config_path: String,
}

impl ProcessManagerService {
    pub fn new(mgr: ProcessManager, config_path: String) -> Self {
        Self {
            mgr,
            started_at: Instant::now(),
            config_path,
        }
    }
}

#[tonic::async_trait]
impl proto::process_manager_server::ProcessManager for ProcessManagerService {
    async fn list(
        &self,
        _request: Request<proto::ListRequest>,
    ) -> Result<Response<proto::ListResponse>, Status> {
        let procs = self.mgr.read().await;
        let processes = procs.iter().map(process_to_proto).collect();
        Ok(Response::new(proto::ListResponse { processes }))
    }

    async fn describe(
        &self,
        request: Request<proto::DescribeRequest>,
    ) -> Result<Response<proto::DescribeResponse>, Status> {
        let name = request.into_inner().name;
        let procs = self.mgr.read().await;
        let proc = procs
            .iter()
            .find(|p| p.name == name)
            .ok_or_else(|| Status::not_found(format!("process '{name}' not found")))?;

        Ok(Response::new(proto::DescribeResponse {
            detail: Some(process_detail(proc)),
        }))
    }

    async fn get_status(
        &self,
        _request: Request<proto::GetStatusRequest>,
    ) -> Result<Response<proto::GetStatusResponse>, Status> {
        let procs = self.mgr.read().await;
        let total = procs.len() as u32;
        let running = procs.iter().filter(|p| p.is_running()).count() as u32;
        let stopped = procs
            .iter()
            .filter(|p| p.state() == ProcessState::Stopped)
            .count() as u32;
        let failed = procs
            .iter()
            .filter(|p| p.state() == ProcessState::Failed)
            .count() as u32;
        let created = procs
            .iter()
            .filter(|p| p.state() == ProcessState::Created)
            .count() as u32;
        let exited = procs
            .iter()
            .filter(|p| p.state() == ProcessState::Exited)
            .count() as u32;

        Ok(Response::new(proto::GetStatusResponse {
            ready: true,
            version: env!("CARGO_PKG_VERSION").to_string(),
            uptime_seconds: self.started_at.elapsed().as_secs(),
            total_processes: total,
            running_processes: running,
            stopped_processes: stopped,
            failed_processes: failed,
            created_processes: created,
            exited_processes: exited,
            config_path: self.config_path.clone(),
        }))
    }
}

fn state_to_proto(state: ProcessState) -> i32 {
    match state {
        ProcessState::Created => proto::ProcessState::Created.into(),
        ProcessState::Running => proto::ProcessState::Running.into(),
        ProcessState::Exited => proto::ProcessState::Exited.into(),
        ProcessState::Failed => proto::ProcessState::Failed.into(),
        ProcessState::Stopped => proto::ProcessState::Stopped.into(),
    }
}

fn process_to_proto(proc: &ManagedProcess) -> proto::Process {
    let cfg = proc.config();
    proto::Process {
        name: proc.name.clone(),
        pid: proc.pid().unwrap_or(0),
        command: cfg.command.clone(),
        args: cfg.args.clone(),
        state: state_to_proto(proc.state()),
    }
}

fn process_detail(proc: &ManagedProcess) -> proto::ProcessDetail {
    let cfg = proc.config();
    proto::ProcessDetail {
        name: proc.name.clone(),
        description: cfg.description.clone().unwrap_or_default(),
        pid: proc.pid().unwrap_or(0),
        state: state_to_proto(proc.state()),
        command: cfg.command.clone(),
        args: cfg.args.clone(),
        working_dir: cfg.working_dir.clone().unwrap_or_default(),
        env: cfg.env.clone(),
        restart_policy: format!("{:?}", cfg.restart),
        stdout: cfg.stdout.clone(),
        stderr: cfg.stderr.clone(),
        auto_start: cfg.auto_start,
        condition_path_exists: cfg.condition_path_exists.clone().unwrap_or_default(),
        after: cfg.after.clone(),
        before: cfg.before.clone(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ProcessConfig;

    #[test]
    fn test_state_to_proto_mapping() {
        assert_eq!(
            state_to_proto(ProcessState::Created),
            proto::ProcessState::Created as i32
        );
        assert_eq!(
            state_to_proto(ProcessState::Running),
            proto::ProcessState::Running as i32
        );
        assert_eq!(
            state_to_proto(ProcessState::Exited),
            proto::ProcessState::Exited as i32
        );
        assert_eq!(
            state_to_proto(ProcessState::Failed),
            proto::ProcessState::Failed as i32
        );
        assert_eq!(
            state_to_proto(ProcessState::Stopped),
            proto::ProcessState::Stopped as i32
        );
    }

    #[test]
    fn test_process_to_proto() {
        let cfg = ProcessConfig {
            command: "/usr/bin/sleep".to_string(),
            args: vec!["60".to_string()],
            ..Default::default()
        };
        let proc = ManagedProcess::new("test-proc".to_string(), cfg);
        let proto = process_to_proto(&proc);
        assert_eq!(proto.name, "test-proc");
        assert_eq!(proto.command, "/usr/bin/sleep");
        assert_eq!(proto.args, vec!["60"]);
        assert_eq!(proto.pid, 0);
        assert_eq!(proto.state, proto::ProcessState::Created as i32);
    }

    #[test]
    fn test_process_detail() {
        let cfg = ProcessConfig {
            command: "/usr/bin/test".to_string(),
            description: Some("A test process".to_string()),
            working_dir: Some("/tmp".to_string()),
            auto_start: false,
            after: vec!["dep-a".to_string()],
            before: vec!["dep-b".to_string()],
            ..Default::default()
        };
        let proc = ManagedProcess::new("detail-proc".to_string(), cfg);
        let detail = process_detail(&proc);
        assert_eq!(detail.name, "detail-proc");
        assert_eq!(detail.description, "A test process");
        assert_eq!(detail.working_dir, "/tmp");
        assert!(!detail.auto_start);
        assert_eq!(detail.after, vec!["dep-a"]);
        assert_eq!(detail.before, vec!["dep-b"]);
    }

    #[tokio::test]
    async fn test_process_to_proto_running_with_pid() {
        let cfg = ProcessConfig {
            command: "/bin/sleep".to_string(),
            args: vec!["60".to_string()],
            ..Default::default()
        };
        let mut proc = ManagedProcess::new("sleeper".to_string(), cfg);
        proc.spawn().unwrap();

        let proto = process_to_proto(&proc);
        assert_eq!(proto.name, "sleeper");
        assert_eq!(proto.state, proto::ProcessState::Running as i32);
        assert!(proto.pid > 0, "running process should have a non-zero pid");
        assert_eq!(proto.command, "/bin/sleep");
        assert_eq!(proto.args, vec!["60"]);

        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(proto.pid as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    #[tokio::test]
    async fn test_process_to_proto_failed() {
        let cfg = ProcessConfig {
            command: "/bin/sh".to_string(),
            args: vec!["-c".to_string(), "exit 1".to_string()],
            ..Default::default()
        };
        let mut proc = ManagedProcess::new("fail-proc".to_string(), cfg);
        proc.spawn().unwrap();

        let mut child = proc.take_child().unwrap();
        let status = child.wait().await.unwrap();
        proc.set_last_status(status);

        let proto = process_to_proto(&proc);
        assert_eq!(proto.state, proto::ProcessState::Failed as i32);
        assert!(proto.pid > 0, "failed process retains its last known pid");
    }
}
