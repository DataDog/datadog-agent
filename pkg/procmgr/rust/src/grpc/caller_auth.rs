// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use tonic::{Request, Status};

#[cfg(windows)]
use crate::transport::PipeCallerAuth;

#[cfg(unix)]
use log::debug;

#[derive(Clone, Copy, Debug)]
pub(crate) enum MutatingRpc {
    Create,
    Start,
    Stop,
    ReloadConfig,
}

pub(crate) struct CallerPolicy {
    // Unix reads this. Windows authorizes the pipe client and never touches it.
    #[cfg_attr(not(unix), allow(dead_code))]
    enforce: bool,
}

impl CallerPolicy {
    #[cfg(unix)]
    pub(crate) fn from_process() -> Self {
        Self {
            enforce: nix::unistd::Uid::effective().is_root(),
        }
    }

    #[cfg(not(unix))]
    pub(crate) fn from_process() -> Self {
        Self { enforce: false }
    }

    #[cfg(all(test, unix))]
    pub(crate) fn from_enforce(enforce: bool) -> Self {
        Self { enforce }
    }
}

#[cfg(unix)]
pub(crate) fn peer_uid<T>(request: &Request<T>) -> Option<u32> {
    request
        .extensions()
        .get::<tonic::transport::server::UdsConnectInfo>()
        .and_then(|info| info.peer_cred.as_ref().map(|cred| cred.uid()))
}

#[cfg(any(unix, test))]
fn peer_may_mutate(enforce: bool, peer_uid: Option<u32>) -> bool {
    if !enforce {
        return true;
    }
    peer_uid == Some(0)
}

#[cfg(windows)]
pub(crate) fn require_mutating_caller<T>(
    request: &Request<T>,
    rpc: MutatingRpc,
    policy: &CallerPolicy,
) -> Result<(), Status> {
    // Same signature as Unix. Windows still checks the pipe client.
    let _ = (rpc, policy);
    let may_mutate = request
        .extensions()
        .get::<PipeCallerAuth>()
        .map(PipeCallerAuth::may_mutate)
        .unwrap_or(false);
    if !may_mutate {
        return Err(Status::permission_denied(
            "operation requires an Administrator, LocalSystem, or installed Agent user pipe client",
        ));
    }
    Ok(())
}

#[cfg(unix)]
pub(crate) fn require_mutating_caller<T>(
    request: &Request<T>,
    rpc: MutatingRpc,
    policy: &CallerPolicy,
) -> Result<(), Status> {
    let uid = peer_uid(request);
    if peer_may_mutate(policy.enforce, uid) {
        return Ok(());
    }
    debug!("denied {rpc:?}: peer uid {uid:?} is not root");
    Err(Status::permission_denied("operation requires root"))
}

// No shipped target is neither Windows nor Unix.
#[cfg(not(any(windows, unix)))]
pub(crate) fn require_mutating_caller<T>(
    _request: &Request<T>,
    _rpc: MutatingRpc,
    _policy: &CallerPolicy,
) -> Result<(), Status> {
    Err(Status::permission_denied("operation requires root"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn peer_may_mutate_allows_only_root_when_enforcing() {
        let non_root = 501;
        let cases = [
            (false, Some(0), true),
            (false, Some(non_root), true),
            (false, None, true),
            (true, Some(0), true),
            (true, Some(non_root), false),
            (true, None, false),
        ];
        for (enforce, uid, allowed) in cases {
            assert_eq!(
                peer_may_mutate(enforce, uid),
                allowed,
                "enforce={enforce} peer_uid={uid:?}"
            );
        }
    }

    #[cfg(unix)]
    #[test]
    fn peer_uid_reads_socketpair_euid_and_missing_creds() {
        // `UnixStream::pair` registers both ends with the current reactor.
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_io()
            .build()
            .unwrap();
        let _guard = runtime.enter();

        let euid = nix::unistd::Uid::effective().as_raw();
        let (left, _right) = tokio::net::UnixStream::pair().unwrap();
        let cred = left.peer_cred().unwrap();

        let mut request = Request::new(());
        request
            .extensions_mut()
            .insert(tonic::transport::server::UdsConnectInfo {
                peer_addr: None,
                peer_cred: Some(cred),
            });
        assert_eq!(peer_uid(&request), Some(euid));

        let mut request = Request::new(());
        request
            .extensions_mut()
            .insert(tonic::transport::server::UdsConnectInfo {
                peer_addr: None,
                peer_cred: None,
            });
        assert_eq!(peer_uid(&request), None);

        let request = Request::new(());
        assert_eq!(peer_uid(&request), None);
    }
}
