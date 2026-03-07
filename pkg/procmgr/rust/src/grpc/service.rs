// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::ProcessManager;
use crate::command::Command;
use crate::config::{ProcessConfig, RestartPolicy};
use crate::grpc::proto;
use crate::process::ManagedProcess;
use crate::state::ProcessState;
use std::time::Instant;
use tokio::sync::{mpsc, oneshot};
use tonic::{Request, Response, Status};

pub struct ProcessManagerService {
    mgr: ProcessManager,
    started_at: Instant,
    config_path: String,
    cmd_tx: mpsc::Sender<Command>,
}

impl ProcessManagerService {
    pub fn new(mgr: ProcessManager, config_path: String, cmd_tx: mpsc::Sender<Command>) -> Self {
        Self {
            mgr,
            started_at: Instant::now(),
            config_path,
            cmd_tx,
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
        let (mut created, mut starting, mut running, mut stopping) = (0u32, 0, 0, 0);
        let (mut stopped, mut failed, mut exited) = (0u32, 0, 0);
        for p in procs.iter() {
            match p.state() {
                ProcessState::Created => created += 1,
                ProcessState::Starting => starting += 1,
                ProcessState::Running => running += 1,
                ProcessState::Stopping => stopping += 1,
                ProcessState::Stopped => stopped += 1,
                ProcessState::Failed => failed += 1,
                ProcessState::Exited => exited += 1,
            }
        }

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
            starting_processes: starting,
            stopping_processes: stopping,
            config_path: self.config_path.clone(),
        }))
    }

    async fn create(
        &self,
        request: Request<proto::CreateRequest>,
    ) -> Result<Response<proto::CreateResponse>, Status> {
        let req = request.into_inner();
        let config = create_request_to_config(&req)?;
        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Command::Create {
                name: req.name,
                config: Box::new(config),
                reply: reply_tx,
            })
            .await
            .map_err(|_| Status::internal("event loop not available"))?;
        reply_rx
            .await
            .map_err(|_| Status::internal("event loop dropped reply"))?
            .map(|()| Response::new(proto::CreateResponse {}))
    }

    async fn start(
        &self,
        request: Request<proto::StartRequest>,
    ) -> Result<Response<proto::StartResponse>, Status> {
        let name = request.into_inner().name;
        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Command::Start {
                name,
                reply: reply_tx,
            })
            .await
            .map_err(|_| Status::internal("event loop not available"))?;
        reply_rx
            .await
            .map_err(|_| Status::internal("event loop dropped reply"))?
            .map(|()| Response::new(proto::StartResponse {}))
    }

    async fn stop(
        &self,
        request: Request<proto::StopRequest>,
    ) -> Result<Response<proto::StopResponse>, Status> {
        let name = request.into_inner().name;
        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Command::Stop {
                name,
                reply: reply_tx,
            })
            .await
            .map_err(|_| Status::internal("event loop not available"))?;
        reply_rx
            .await
            .map_err(|_| Status::internal("event loop dropped reply"))?
            .map(|()| Response::new(proto::StopResponse {}))
    }

    async fn reload_config(
        &self,
        _request: Request<proto::ReloadConfigRequest>,
    ) -> Result<Response<proto::ReloadConfigResponse>, Status> {
        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Command::ReloadConfig { reply: reply_tx })
            .await
            .map_err(|_| Status::internal("event loop not available"))?;
        let result = reply_rx
            .await
            .map_err(|_| Status::internal("event loop dropped reply"))??;
        Ok(Response::new(proto::ReloadConfigResponse {
            added: result.added,
            removed: result.removed,
            unchanged: result.unchanged,
        }))
    }
}

impl From<ProcessState> for proto::ProcessState {
    fn from(state: ProcessState) -> Self {
        match state {
            ProcessState::Created => Self::Created,
            ProcessState::Starting => Self::Starting,
            ProcessState::Running => Self::Running,
            ProcessState::Stopping => Self::Stopping,
            ProcessState::Exited => Self::Exited,
            ProcessState::Failed => Self::Failed,
            ProcessState::Stopped => Self::Stopped,
        }
    }
}

fn process_to_proto(proc: &ManagedProcess) -> proto::Process {
    let cfg = proc.config();
    proto::Process {
        name: proc.name.clone(),
        pid: proc.pid().unwrap_or(0),
        command: cfg.command.clone(),
        args: cfg.args.clone(),
        state: proto::ProcessState::from(proc.state()).into(),
    }
}

#[allow(clippy::result_large_err)]
fn parse_restart_policy(s: &str) -> Result<RestartPolicy, Status> {
    match s {
        "" | "never" | "Never" => Ok(RestartPolicy::Never),
        "always" | "Always" => Ok(RestartPolicy::Always),
        "on-failure" | "OnFailure" => Ok(RestartPolicy::OnFailure),
        "on-success" | "OnSuccess" => Ok(RestartPolicy::OnSuccess),
        other => Err(Status::invalid_argument(format!(
            "unknown restart_policy '{other}'"
        ))),
    }
}

#[allow(clippy::result_large_err)]
fn create_request_to_config(req: &proto::CreateRequest) -> Result<ProcessConfig, Status> {
    let defaults = ProcessConfig::default();
    Ok(ProcessConfig {
        command: req.command.clone(),
        args: req.args.clone(),
        env: req.env.clone(),
        description: if req.description.is_empty() {
            None
        } else {
            Some(req.description.clone())
        },
        working_dir: if req.working_dir.is_empty() {
            None
        } else {
            Some(req.working_dir.clone())
        },
        stdout: if req.stdout.is_empty() {
            defaults.stdout
        } else {
            req.stdout.clone()
        },
        stderr: if req.stderr.is_empty() {
            defaults.stderr
        } else {
            req.stderr.clone()
        },
        auto_start: req.auto_start.unwrap_or(defaults.auto_start),
        restart: parse_restart_policy(&req.restart_policy)?,
        condition_path_exists: if req.condition_path_exists.is_empty() {
            None
        } else {
            Some(req.condition_path_exists.clone())
        },
        after: req.after.clone(),
        before: req.before.clone(),
        ..defaults
    })
}

fn process_detail(proc: &ManagedProcess) -> proto::ProcessDetail {
    let cfg = proc.config();
    proto::ProcessDetail {
        name: proc.name.clone(),
        description: cfg.description.clone().unwrap_or_default(),
        pid: proc.pid().unwrap_or(0),
        state: proto::ProcessState::from(proc.state()).into(),
        command: cfg.command.clone(),
        args: cfg.args.clone(),
        working_dir: cfg.working_dir.clone().unwrap_or_default(),
        env: cfg.env.clone(),
        restart_policy: cfg.restart.to_string(),
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
            proto::ProcessState::from(ProcessState::Created),
            proto::ProcessState::Created,
        );
        assert_eq!(
            proto::ProcessState::from(ProcessState::Starting),
            proto::ProcessState::Starting,
        );
        assert_eq!(
            proto::ProcessState::from(ProcessState::Running),
            proto::ProcessState::Running,
        );
        assert_eq!(
            proto::ProcessState::from(ProcessState::Stopping),
            proto::ProcessState::Stopping,
        );
        assert_eq!(
            proto::ProcessState::from(ProcessState::Exited),
            proto::ProcessState::Exited,
        );
        assert_eq!(
            proto::ProcessState::from(ProcessState::Failed),
            proto::ProcessState::Failed,
        );
        assert_eq!(
            proto::ProcessState::from(ProcessState::Stopped),
            proto::ProcessState::Stopped,
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
