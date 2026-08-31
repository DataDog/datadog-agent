// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use dd_procmgr_client::proto as procmgr;
use dd_procmgr_client::proto::process_manager_server::{ProcessManager, ProcessManagerServer};
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use tonic::{Request, Response, Status};

/// A `None` state represents a missing process definition.
#[derive(Default)]
pub struct FakeProcmgr {
    state: Mutex<Option<i32>>,
    start_result: Mutex<Option<Status>>,
    starts: Mutex<Vec<String>>,
    describes: Mutex<usize>,
    hang: bool,
}

impl FakeProcmgr {
    pub fn in_state(state: procmgr::ProcessState) -> Arc<Self> {
        Arc::new(Self {
            state: Mutex::new(Some(state as i32)),
            ..Default::default()
        })
    }

    pub fn vanished() -> Arc<Self> {
        Arc::new(Self::default())
    }

    pub fn unresponsive() -> Arc<Self> {
        Arc::new(Self {
            hang: true,
            ..Default::default()
        })
    }

    pub fn failing_start(state: procmgr::ProcessState, status: Status) -> Arc<Self> {
        Arc::new(Self {
            state: Mutex::new(Some(state as i32)),
            start_result: Mutex::new(Some(status)),
            ..Default::default()
        })
    }

    pub fn started(&self) -> Vec<String> {
        self.starts.lock().unwrap().clone()
    }

    pub fn describe_count(&self) -> usize {
        *self.describes.lock().unwrap()
    }
}

/// Local newtype required by the orphan rule when Bazel supplies foreign bindings.
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
        *self.0.describes.lock().unwrap() += 1;
        let state = self
            .0
            .state
            .lock()
            .unwrap()
            .ok_or_else(|| Status::not_found("process not found"))?;
        Ok(Response::new(procmgr::DescribeResponse {
            detail: Some(procmgr::ProcessDetail {
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
        match self.0.start_result.lock().unwrap().take() {
            Some(status) => Err(status),
            None => Ok(Response::new(procmgr::StartResponse::default())),
        }
    }

    async fn stop(
        &self,
        _: Request<procmgr::StopRequest>,
    ) -> Result<Response<procmgr::StopResponse>, Status> {
        Ok(Response::new(procmgr::StopResponse::default()))
    }

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
