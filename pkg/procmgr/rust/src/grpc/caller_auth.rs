// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use tonic::{Request, Status};

#[cfg(windows)]
use crate::transport::PipeCallerAuth;

/// `Create` requires an Administrator or LocalSystem pipe client on Windows.
///
/// `Start`, `Stop`, and `ReloadConfig` stay open to the agent user for now (dd-procmgr CLI
/// and current supervision flows); revisit tightening in a follow-up.
#[cfg(windows)]
pub(crate) fn require_privileged_pipe_client<T>(request: &Request<T>) -> Result<(), Status> {
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
pub(crate) fn require_privileged_pipe_client<T>(_request: &Request<T>) -> Result<(), Status> {
    Ok(())
}
