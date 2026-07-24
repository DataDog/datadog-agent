use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result, anyhow};
use serde_json::Value;
use shlib_core::*;

use crate::backend;
use crate::config::{CheckConfig, SubTask};
use crate::constants::SDS_RESULT_EVENT_TYPE;
use crate::payload::{Match, ScanStatus};
use crate::proto::{self, ScanMetadata, ScanResult, ScanTaskMetadata, SdsResultPayload, Status};
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

    // Time the scan so the payload can carry started_at / ended_at / duration.
    let started_at = SystemTime::now();
    let scan = run_scan(scanner, sub_task);
    let ended_at = SystemTime::now();

    // A sub task failure is reported inside the payload (status=ERROR) rather
    // than aborting the check, so every sub task produces exactly one event.
    let (status, failure_reason, matches, scanned_row_count) = match scan {
        Ok(out) => {
            println!(
                "datasecurity: sub task succeeded ({} match(es))",
                out.matches.len()
            );
            (
                ScanStatus::Success,
                String::new(),
                out.matches,
                out.scanned_row_count,
            )
        }
        Err(err) => {
            let reason = format!("{err:#}");
            eprintln!(
                "datasecurity: sub task {} failed: {reason}",
                sub_task.sub_task_id
            );
            (ScanStatus::Error, reason, Vec::new(), 0)
        }
    };

    // Build the SDS result protobuf: task metadata, timing, postgres location and
    // matches, mirroring the Data Observability crawler payload.
    let payload = build_sds_result(
        config,
        sub_task,
        status,
        &failure_reason,
        &matches,
        scanned_row_count,
        started_at,
        ended_at,
    );

    // Emit the protobuf on the `sds-result` event platform track.
    if config.send_sds_result {
        check.event_platform_event_bytes(&proto::encode(&payload), SDS_RESULT_EVENT_TYPE)?;
    }

    Ok(())
}

/// Builds the `SdsResultPayload` protobuf for one sub task.
///
/// Mirrors the Data Observability crawler payload (`Resource`, `RuleIds`,
/// `ScanningSource`, `ScanResults`), swapping the snowflake location for a
/// postgres one and adding the scan-task metadata block.
#[allow(clippy::too_many_arguments)]
fn build_sds_result(
    config: &CheckConfig,
    sub_task: &SubTask,
    status: ScanStatus,
    failure_reason: &str,
    matches: &[Match],
    scanned_row_count: i64,
    started_at: SystemTime,
    ended_at: SystemTime,
) -> SdsResultPayload {
    let entity = &sub_task.entity;
    let duration_ms = ended_at
        .duration_since(started_at)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0);

    let location = proto::ScanLocation {
        scan_location: Some(proto::scan_location::ScanLocation::PostgresTable(
            proto::PostgresTable {
                database_cluster_name: entity.database_cluster_name.clone(),
                database_instance_name: entity.database_instance_name.clone(),
                database_host_name: sub_task.connection.host.clone(),
                database_name: entity.database.clone(),
                schema_name: entity.schema.clone(),
                table_name: entity.table.clone(),
                scanned_row_count,
                // TODO(DSEC): populate table_row_count (from DBM metadata) and
                // scanned_columns.
                ..Default::default()
            },
        )),
        ..Default::default()
    };

    let scan_result = ScanResult {
        table_matches: proto::table_matches(matches),
        location: Some(location),
        duration: duration_ms,
        scan_metadata: Some(ScanMetadata {
            scan_task_metadata: Some(ScanTaskMetadata {
                task_id: config.task_id.clone(),
                sub_task_id: sub_task.sub_task_id.clone(),
                started_at: Some(proto::to_timestamp(started_at)),
                ended_at: Some(proto::to_timestamp(ended_at)),
                status: match status {
                    ScanStatus::Success => Status::Success,
                    ScanStatus::Error => Status::Error,
                } as i32,
                failure_reason: (!failure_reason.is_empty()).then(|| failure_reason.to_string()),
            }),
        }),
        ..Default::default()
    };

    SdsResultPayload {
        timestamp: now_unix_millis(),
        resource: Some(proto::Resource {
            r#type: "postgres_table".to_string(),
            name: resource_name(sub_task),
        }),
        rule_ids: config
            .scanning_rules
            .iter()
            .map(|rule| rule.id.clone())
            .collect(),
        // The scanning source is the Agent. TODO(DSEC): populate hostname and
        // agent version once the check receives them (not provided via config yet).
        scanning_source: Some(proto::ScanningSource {
            source: Some(proto::scanning_source::Source::Agent(
                proto::scanning_source::Agent::default(),
            )),
        }),
        scan_results: vec![scan_result],
        ..Default::default()
    }
}

/// Result of a successful sub task scan.
struct ScanOutput {
    matches: Vec<Match>,
    scanned_row_count: i64,
}

/// Fetches the sub task's data and scans it, returning the matches and the
/// number of rows scanned.
fn run_scan(scanner: &Scanner, sub_task: &SubTask) -> Result<ScanOutput> {
    let data = backend::fetch_data(sub_task).context("fetching sub task data")?;
    let matches = scanner
        .scan(data.clone())
        .context("scanning sub task data")?;
    Ok(ScanOutput {
        scanned_row_count: scanned_rows(&data),
        matches,
    })
}

/// Number of rows scanned: the longest column array in the `{ column: [values] }`
/// map returned by the backend.
fn scanned_rows(data: &Value) -> i64 {
    data.as_object()
        .and_then(|columns| {
            columns
                .values()
                .filter_map(|value| value.as_array().map(Vec::len))
                .max()
        })
        .unwrap_or(0) as i64
}

/// Current unix time in milliseconds, for the payload timestamp.
fn now_unix_millis() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

/// Resource name (`<instance_name>.<database>.<schema>.<table>`), following the
/// DO crawler convention.
fn resource_name(sub_task: &SubTask) -> String {
    let entity = &sub_task.entity;
    format!(
        "{}.{}.{}.{}",
        entity.database_instance_name, entity.database, entity.schema, entity.table
    )
}
