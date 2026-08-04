use anyhow::{Context, Result, anyhow};
use shlib_core::*;

use crate::backend;
use crate::config::{CheckConfig, SubTask};
use crate::constants::SDS_RESULT_EVENT_TYPE;
use crate::proto::{self, Status as ScanStatus};
use crate::result::{ScanOutcome, build_sds_result};
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
    check.log(
        LogLevel::Info,
        &format!(
            "datasecurity: check started (task_id={}, {} rule(s), {} sub task(s))",
            config.task_id,
            config.scanning_rules.len(),
            config.scan_data.len()
        ),
    );

    let scanner = Scanner::new(&config.scanning_rules).context("failed to create sds scanner")?;

    for sub_task in &config.scan_data {
        run_sub_task(check, &config, &scanner, sub_task)?;
    }

    check.log(LogLevel::Info, "datasecurity: check completed");
    Ok(())
}

fn run_sub_task(
    check: &AgentCheck,
    config: &CheckConfig,
    scanner: &Scanner,
    sub_task: &SubTask,
) -> Result<()> {
    check.log(
        LogLevel::Info,
        &format!(
            "datasecurity: running sub task (sub_task_id={}, platform={})",
            sub_task.sub_task_id, sub_task.entity.platform
        ),
    );

    // TODO(DSEC-180): time the scan and populate task metadata started_at / ended_at
    // A sub task failure is reported inside the payload (status=ERROR) rather
    // than aborting the check, so every sub task produces exactly one event.
    let (status, failure_reason, outcome) = match run_scan(scanner, sub_task) {
        Ok(outcome) => {
            check.log(
                LogLevel::Info,
                &format!(
                    "datasecurity: sub task succeeded ({} match(es))",
                    outcome.matches.len()
                ),
            );
            (ScanStatus::Success, String::new(), outcome)
        }
        Err(err) => {
            let reason = format!("{err:#}");
            check.log(
                LogLevel::Error,
                &format!(
                    "datasecurity: sub task {} failed: {reason}",
                    sub_task.sub_task_id
                ),
            );
            (ScanStatus::Error, reason, ScanOutcome::default())
        }
    };

    // Build the SDS result protobuf for this sub task.
    let payload = build_sds_result(config, sub_task, status, &failure_reason, outcome);

    // Emit the protobuf on the `sds-result` event platform track.
    check.event_platform_event_bytes(&proto::encode(&payload), SDS_RESULT_EVENT_TYPE)?;

    Ok(())
}

/// Fetches the sub task's data and scans it, returning the matches and the
/// scanned-table statistics.
/// TODO(dsec-161): add tests for the scan.
fn run_scan(scanner: &Scanner, sub_task: &SubTask) -> Result<ScanOutcome> {
    let data = backend::fetch_data(sub_task).context("fetching sub task data")?;
    let matches = scanner
        .scan(data.columns)
        .context("scanning sub task data")?;
    Ok(ScanOutcome {
        matches,
        scanned_columns: data.scanned_columns,
        scanned_row_count: data.scanned_row_count,
    })
}
