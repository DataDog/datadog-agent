use anyhow::{Context, Result, anyhow};
use core::*;

use crate::backend;
use crate::config::{CheckConfig, SubTask};
use crate::payload::{Match, ScanEventPayload, ScanStatus};
use crate::scanning::Scanner;

/// Check entrypoint.
///
/// Flattens the full error chain into one message (`{e:#}`) so the reason is
/// shown before leaving the check.
pub fn check(check: &AgentCheck) -> Result<()> {
    run(check).map_err(|e| anyhow!("{e:#}"))
}

/// Check implementation.
fn run(check: &AgentCheck) -> Result<()> {
    let config = CheckConfig::from_instance(check)?;
    println!(
        "datasecurity: check started (task_id={}, {} rule(s), {} sub task(s))",
        config.task_id,
        config.scanning_rules.len(),
        config.scan_data.len()
    );

    let scanner = Scanner::new(&config.scanning_rules).context("failed to create sds scanner")?;

    for sub_task in &config.scan_data {
        run_sub_task(check, &config, &scanner, sub_task)?;
    }

    println!("datasecurity: check completed");
    Ok(())
}

fn run_sub_task(
    check: &AgentCheck,
    config: &CheckConfig,
    scanner: &Scanner,
    sub_task: &SubTask,
) -> Result<()> {
    println!(
        "datasecurity: running sub task (sub_task_id={}, platform={})",
        sub_task.sub_task_id, sub_task.entity.platform
    );

    // A sub task failure is reported inside the payload (status=ERROR) rather
    // than aborting the check, so every sub task produces exactly one event.
    let (status, failure_reason, matches) = match run_scan(scanner, sub_task) {
        Ok(matches) => {
            println!(
                "datasecurity: sub task succeeded ({} match(es))",
                matches.len()
            );
            (ScanStatus::Success, String::new(), matches)
        }
        Err(err) => {
            let reason = format!("{err:#}");
            eprintln!(
                "datasecurity: sub task {} failed: {reason}",
                sub_task.sub_task_id
            );
            (ScanStatus::Error, reason, Vec::new())
        }
    };

    let payload = ScanEventPayload {
        task_id: config.task_id.clone(),
        sub_task_id: sub_task.sub_task_id.clone(),
        status,
        failure_reason,
        matches,
    };

    // TODO(DSEC-140): send sdsresult rather than an event
    let payload_json =
        serde_json::to_string(&payload).context("failed to serialize scan event payload")?;
    check.event(
        "datasecurity scan result",
        &payload_json,
        0,
        "normal",
        "",
        &[],
        "info",
        "",
        "datasecurity",
        "",
    )?;

    Ok(())
}

/// Fetches the sub task's data and scans it, returning the matches.
/// TODO(dsec-161): add tests for the scan.
fn run_scan(scanner: &Scanner, sub_task: &SubTask) -> Result<Vec<Match>> {
    let data = backend::fetch_data(sub_task).context("fetching sub task data")?;
    scanner.scan(&data).context("scanning sub task data")
}
