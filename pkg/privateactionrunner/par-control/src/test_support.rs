// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! An in-process stand-in for `dd-procmgrd`, for tests that need to exercise the
//! real gRPC client rather than a hand-rolled fake of our own trait.
//!
//! Deliberately returns a socket path rather than a built client: the client
//! constructor changes as par-control gains configuration, and the harness
//! should not have to change with it.

use crate::proto::procmgr;
use crate::proto::procmgr::process_manager_server::{ProcessManager, ProcessManagerServer};
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use tonic::{Request, Response, Status};

/// Scripted process-manager behavior.
///
/// `states` is consumed one entry per `Describe` so a test can drive a state
/// sequence; the final entry sticks, so a single-element script means "always
/// this state". `None` means the process manager reports no detail at all, i.e.
/// the definition has vanished.
#[derive(Default)]
pub struct FakeProcmgr {
    states: Mutex<Vec<Option<i32>>>,
    start_result: Mutex<Option<Status>>,
    stop_result: Mutex<Option<Status>>,
    starts: Mutex<Vec<String>>,
    stops: Mutex<Vec<String>>,
    /// Accept the connection but never answer, to exercise RPC deadlines.
    hang: bool,
}

impl FakeProcmgr {
    /// Always report `state`.
    pub fn in_state(state: procmgr::ProcessState) -> Arc<Self> {
        Arc::new(Self {
            states: Mutex::new(vec![Some(state as i32)]),
            ..Default::default()
        })
    }

    /// Report each state in turn, then stay in the last one.
    pub fn in_states(states: &[procmgr::ProcessState]) -> Arc<Self> {
        Arc::new(Self {
            states: Mutex::new(states.iter().map(|s| Some(*s as i32)).collect()),
            ..Default::default()
        })
    }

    /// Report no process detail: the definition is gone.
    pub fn vanished() -> Arc<Self> {
        Arc::new(Self {
            states: Mutex::new(vec![None]),
            ..Default::default()
        })
    }

    /// Accept connections but never answer any RPC.
    pub fn unresponsive() -> Arc<Self> {
        Arc::new(Self {
            hang: true,
            ..Default::default()
        })
    }

    /// Fail `Start` with `status`, reporting `state` beforehand.
    pub fn failing_start(state: procmgr::ProcessState, status: Status) -> Arc<Self> {
        Arc::new(Self {
            states: Mutex::new(vec![Some(state as i32)]),
            start_result: Mutex::new(Some(status)),
            ..Default::default()
        })
    }

    /// Process names passed to `Start`, in order.
    pub fn started(&self) -> Vec<String> {
        self.starts.lock().unwrap().clone()
    }

    /// Process names passed to `Stop`, in order.
    pub fn stopped(&self) -> Vec<String> {
        self.stops.lock().unwrap().clone()
    }

    fn next_state(&self) -> Option<i32> {
        let mut states = self.states.lock().unwrap();
        if states.len() > 1 {
            states.remove(0)
        } else {
            states.first().copied().flatten()
        }
    }
}

/// Newtype so the trait impl has a local self type.
///
/// Under Bazel the generated bindings live in a foreign crate, so
/// `impl ProcessManager for Arc<FakeProcmgr>` would break the orphan rule even
/// though it compiles under `cargo`, where `include_proto!` generates the trait
/// locally.
#[derive(Clone)]
pub struct FakeService(pub Arc<FakeProcmgr>);

#[tonic::async_trait]
impl ProcessManager for FakeService {
    async fn describe(
        &self,
        _: Request<procmgr::DescribeRequest>,
    ) -> Result<Response<procmgr::DescribeResponse>, Status> {
        if self.0.hang {
            std::future::pending::<()>().await;
        }
        Ok(Response::new(procmgr::DescribeResponse {
            detail: self.0.next_state().map(|state| procmgr::ProcessDetail {
                state,
                ..Default::default()
            }),
        }))
    }

    async fn start(
        &self,
        request: Request<procmgr::StartRequest>,
    ) -> Result<Response<procmgr::StartResponse>, Status> {
        if self.0.hang {
            std::future::pending::<()>().await;
        }
        self.0
            .starts
            .lock()
            .unwrap()
            .push(request.into_inner().name_or_uuid);
        match self.0.start_result.lock().unwrap().clone() {
            Some(status) => Err(status),
            None => Ok(Response::new(procmgr::StartResponse::default())),
        }
    }

    async fn stop(
        &self,
        request: Request<procmgr::StopRequest>,
    ) -> Result<Response<procmgr::StopResponse>, Status> {
        self.0
            .stops
            .lock()
            .unwrap()
            .push(request.into_inner().name_or_uuid);
        match self.0.stop_result.lock().unwrap().clone() {
            Some(status) => Err(status),
            None => Ok(Response::new(procmgr::StopResponse::default())),
        }
    }

    // The remaining RPCs are not part of par-control's contract with the daemon.

    async fn list(
        &self,
        _: Request<procmgr::ListRequest>,
    ) -> Result<Response<procmgr::ListResponse>, Status> {
        Err(Status::unimplemented("list"))
    }

    async fn get_status(
        &self,
        _: Request<procmgr::GetStatusRequest>,
    ) -> Result<Response<procmgr::GetStatusResponse>, Status> {
        Err(Status::unimplemented("get_status"))
    }

    async fn create(
        &self,
        _: Request<procmgr::CreateRequest>,
    ) -> Result<Response<procmgr::CreateResponse>, Status> {
        Err(Status::unimplemented("create"))
    }

    async fn reload_config(
        &self,
        _: Request<procmgr::ReloadConfigRequest>,
    ) -> Result<Response<procmgr::ReloadConfigResponse>, Status> {
        Err(Status::unimplemented("reload_config"))
    }

    async fn get_config(
        &self,
        _: Request<procmgr::GetConfigRequest>,
    ) -> Result<Response<procmgr::GetConfigResponse>, Status> {
        Err(Status::unimplemented("get_config"))
    }
}

/// Serve `fake` on a temporary Unix socket.
///
/// The returned [`tempfile::TempDir`] owns the socket, so callers must keep it
/// alive for the duration of the test.
#[cfg(unix)]
pub async fn serve_procmgr(fake: Arc<FakeProcmgr>) -> (PathBuf, tempfile::TempDir) {
    use tokio_stream::wrappers::UnixListenerStream;

    let dir = tempfile::tempdir().expect("tempdir");
    let socket = dir.path().join("dd-procmgrd.sock");
    let listener = tokio::net::UnixListener::bind(&socket).expect("bind fake procmgr socket");
    tokio::spawn(async move {
        let _ = tonic::transport::Server::builder()
            .add_service(ProcessManagerServer::new(FakeService(fake)))
            .serve_with_incoming(UnixListenerStream::new(listener))
            .await;
    });
    (socket, dir)
}
