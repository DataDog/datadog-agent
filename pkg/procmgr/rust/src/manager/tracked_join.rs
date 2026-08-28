// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::deferred_cleanup;
use crate::shutdown::ShutdownBudget;
use log::warn;
use std::time::Duration;
use tokio::task::JoinHandle;

pub(in crate::manager) enum TrackedJoinTimeout {
    Abort { log_label: &'static str },
    Defer { log_label: &'static str },
}

pub(in crate::manager) async fn join_tracked_handle(
    handle: JoinHandle<()>,
    budget: &ShutdownBudget,
    timeout: TrackedJoinTimeout,
    log_failed: fn(&str, tokio::task::JoinError),
) {
    let log_label = match timeout {
        TrackedJoinTimeout::Abort { log_label } | TrackedJoinTimeout::Defer { log_label } => {
            log_label
        }
    };

    let cap = budget.remaining_cap(Duration::MAX);
    if budget.is_bounded() && cap.is_zero() {
        match timeout {
            TrackedJoinTimeout::Abort { .. } => {
                warn!("{log_label} join budget exhausted; aborting in-flight task");
                handle.abort();
            }
            TrackedJoinTimeout::Defer { .. } => {
                warn!("{log_label} join budget exhausted; deferring task to runtime shutdown");
                deferred_cleanup::register_deferred_spawn_join(handle);
            }
        }
        return;
    }

    if !budget.is_bounded() {
        log_join_result(log_label, handle.await, log_failed);
        return;
    }

    match timeout {
        TrackedJoinTimeout::Abort { .. } => {
            let abort = handle.abort_handle();
            tokio::select! {
                result = handle => log_join_result(log_label, result, log_failed),
                _ = tokio::time::sleep(cap) => {
                    warn!(
                        "timed out waiting for {log_label} ({cap:?} left in service shutdown budget); aborting in-flight task"
                    );
                    abort.abort();
                }
            }
        }
        TrackedJoinTimeout::Defer { .. } => {
            let mut monitor = tokio::spawn(handle);
            tokio::select! {
                result = &mut monitor => log_monitor_result(log_label, result, log_failed),
                _ = tokio::time::sleep(cap) => {
                    warn!(
                        "timed out waiting for {log_label} ({cap:?} left in service shutdown budget); deferring to runtime shutdown"
                    );
                    deferred_cleanup::register_deferred_spawn_join(tokio::spawn(async move {
                        log_monitor_result(log_label, monitor.await, log_failed);
                    }));
                }
            }
        }
    }
}

fn log_join_result(
    log_label: &str,
    result: Result<(), tokio::task::JoinError>,
    log_failed: fn(&str, tokio::task::JoinError),
) {
    match result {
        Ok(()) => {}
        Err(error) if error.is_cancelled() => {}
        Err(error) => log_failed(log_label, error),
    }
}

fn log_monitor_result(
    log_label: &str,
    result: Result<Result<(), tokio::task::JoinError>, tokio::task::JoinError>,
    log_failed: fn(&str, tokio::task::JoinError),
) {
    match result {
        Ok(inner) => log_join_result(log_label, inner, log_failed),
        Err(error) => warn!("{log_label} monitor failed: {error}"),
    }
}
