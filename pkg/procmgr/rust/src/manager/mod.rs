// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#![allow(clippy::result_large_err)]

mod catalog;
mod deferred_cleanup;
mod lifecycle;
mod process_manager;
mod runtime;
mod spawn;
mod startup;
mod supervisor;

#[cfg(test)]
mod tests;

pub(crate) use runtime::RuntimeContext;

use crate::process::ManagedProcess;
use tonic::Status;

pub use process_manager::ProcessManager;
pub use supervisor::Supervisor;

#[cfg(all(test, unix))]
pub(crate) use runtime::spawn_command_loop_for_tests;

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

fn enqueue_pending_restart(proc: &mut ManagedProcess, ctx: &RuntimeContext) {
    if let Some(delay) = proc.schedule_restart() {
        let pending = PendingRestart {
            uuid: proc.uuid().to_owned(),
            spawn_seq: proc.spawn_seq(),
        };
        let tx = ctx.restart_tx.clone();
        tokio::spawn(async move {
            tokio::time::sleep(delay).await;
            let _ = tx.send(pending).await;
        });
    }
}
