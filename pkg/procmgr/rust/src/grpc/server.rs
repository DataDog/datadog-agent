// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::grpc::proto;
use crate::grpc::service::ProcessManagerService;
use crate::manager::{ProcessManager, RuntimeContext};
use crate::transport;
use anyhow::{Context, Result};
use tonic::transport::Server;

pub(crate) async fn run(
    mgr: ProcessManager,
    ctx: RuntimeContext,
    shutdown: tokio::sync::oneshot::Receiver<()>,
) -> Result<()> {
    let svc = ProcessManagerService::new(mgr, ctx.cmd_tx);
    let pm_service = proto::process_manager_server::ProcessManagerServer::new(svc);

    let reflection = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(proto::FILE_DESCRIPTOR_SET)
        .build_v1()
        .context("failed to build gRPC reflection service")?;
    let router = Server::builder()
        .add_service(reflection)
        .add_service(pm_service);

    transport::serve(router, async {
        let _ = shutdown.await;
    })
    .await
}
