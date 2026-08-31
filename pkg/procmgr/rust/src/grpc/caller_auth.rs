// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use tonic::{Request, Status};

#[cfg(windows)]
use crate::transport::PipeCallerAuth;

/// Gate mutating RPCs on the identity of the connected named-pipe client.
///
/// On Windows, each gRPC request arrives over a named pipe. When the server accepts
/// the connection, `transport::named_pipe` wraps the pipe in [`PipeCallerAuth`] and
/// tonic copies that into the request extensions (via `Connected::connect_info`).
///
/// This helper reads that extension and calls [`PipeCallerAuth::may_mutate`], which
/// impersonates the pipe client and checks whether its token is LocalSystem or a
/// member of the built-in Administrators group. If the extension is missing or the
/// client fails that check, the RPC is rejected with `permission_denied`.
#[cfg(windows)]
pub(crate) fn require_mutating_pipe_client<T>(request: &Request<T>) -> Result<(), Status> {
    let may_mutate = request
        .extensions()
        .get::<PipeCallerAuth>()
        .map(PipeCallerAuth::may_mutate)
        .unwrap_or(false);
    if !may_mutate {
        return Err(Status::permission_denied(
            "operation requires an Administrator or LocalSystem pipe client",
        ));
    }
    Ok(())
}

#[cfg(not(windows))]
pub(crate) fn require_mutating_pipe_client<T>(_request: &Request<T>) -> Result<(), Status> {
    Ok(())
}
