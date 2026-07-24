use anyhow::{Context, Result, anyhow};
use shlib_core::*;

use crate::backend;
use crate::config::{CheckConfig, SubTask};
use crate::constants::SDS_RESULT_EVENT_TYPE;
use crate::proto::{self, Status as ScanStatus, TableMatch};
use crate::result::build_sds_result;
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

    // TODO(DSEC-180): time the scan and populate task metadata started_at / ended_at
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

    // Build the SDS result protobuf for this sub task.
    let payload = build_sds_result(config, sub_task, status, &failure_reason, &matches);

    // Emit the protobuf on the `sds-result` event platform track.
    check.event_platform_event_bytes(&proto::encode(&payload), SDS_RESULT_EVENT_TYPE)?;

    Ok(())
}

/// Fetches the sub task's data and scans it, returning the matches.
/// TODO(dsec-161): add tests for the scan.
/// TODO(dsec-179): Return the number of rows scanned and additional info
fn run_scan(scanner: &Scanner, sub_task: &SubTask) -> Result<Vec<TableMatch>> {
    let data = backend::fetch_data(sub_task).context("fetching sub task data")?;
    scanner.scan(data).context("scanning sub task data")
}
