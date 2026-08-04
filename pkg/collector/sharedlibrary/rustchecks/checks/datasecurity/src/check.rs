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

#[cfg(test)]
mod tests {
    use prost::Message;
    use serde_json::json;
    use shlib_core::stubs::AggregatorStub;

    use crate::backend::{ScanData, ScannedColumn, mock};
    use crate::constants::SDS_RESULT_EVENT_TYPE;
    use crate::proto::{
        PostgresScannedColumn, PostgresTable, Resource, ScanLocation, ScanMetadata, ScanResult,
        ScanTaskMetadata, ScanningSource, SdsResultPayload, Status, TableMatch, scan_location,
        scanning_source,
    };

    use super::check;

    const INSTANCE: &str = r#"
task_id: task-1
scanning_rules:
  - id: email
    pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
scan_data:
  - sub_task_id: sub-1
    query: SELECT email FROM users
    timeout_seconds: 5
    connection:
      host: mock
      dbname: app
    entity:
      platform: mock
      database_cluster_name: cluster
      database_instance_name: inst
      database: app
      schema: public
      table: users
"#;

    #[test]
    fn scans_data_and_emits_sds_result() {
        // The mock engine returns this in place of a real query: two scanned
        // columns, `email` (which matches) and `name` (which does not).
        mock::set_data(ScanData {
            columns: json!({
                "email": ["alice@corp.io", "bob@corp.io hatem@corp.io"],
                "name": ["alice", "bob"],
            }),
            scanned_columns: vec![
                ScannedColumn {
                    name: "email".to_string(),
                    data_type: "text".to_string(),
                },
                ScannedColumn {
                    name: "name".to_string(),
                    data_type: "text".to_string(),
                },
            ],
            scanned_row_count: 2,
        });

        let aggregator = AggregatorStub::new();
        check(&aggregator.agent_check("{}", INSTANCE)).expect("check run failed");

        // Exactly one sds-result event platform event is emitted.
        let events = aggregator.event_platform_events();
        assert_eq!(events.len(), 1);
        assert_eq!(events[0].event_type, SDS_RESULT_EVENT_TYPE);

        // The decoded payload matches in full (timestamp aside, it is clock-based).
        let payload =
            SdsResultPayload::decode(events[0].raw_event.as_slice()).expect("payload decodes");
        assert!(payload.timestamp > 0, "timestamp should be populated");

        assert_eq!(
            payload,
            SdsResultPayload {
                timestamp: payload.timestamp,
                resource: Some(Resource {
                    r#type: "postgres_table".to_string(),
                    name: "inst.app.public.users".to_string(),
                }),
                rule_ids: vec!["email".to_string()],
                scanning_source: Some(ScanningSource {
                    source: Some(scanning_source::Source::Agent(
                        scanning_source::Agent::default()
                    )),
                }),
                scan_results: vec![ScanResult {
                    table_matches: vec![TableMatch {
                        rule_id: "email".to_string(),
                        column_name: "email".to_string(),
                        count_matched_rows: 2,
                        count_matches: 3,
                        ..Default::default()
                    }],
                    location: Some(ScanLocation {
                        scan_location: Some(scan_location::ScanLocation::PostgresTable(
                            PostgresTable {
                                database_cluster_name: "cluster".to_string(),
                                database_instance_name: "inst".to_string(),
                                database_host_name: "mock".to_string(),
                                database_name: "app".to_string(),
                                schema_name: "public".to_string(),
                                table_name: "users".to_string(),
                                scanned_row_count: 2,
                                scanned_columns: vec![
                                    PostgresScannedColumn {
                                        name: "email".to_string(),
                                        data_type: "text".to_string(),
                                    },
                                    PostgresScannedColumn {
                                        name: "name".to_string(),
                                        data_type: "text".to_string(),
                                    },
                                ],
                                ..Default::default()
                            }
                        )),
                        ..Default::default()
                    }),
                    scan_metadata: Some(ScanMetadata {
                        scan_task_metadata: Some(ScanTaskMetadata {
                            task_id: "task-1".to_string(),
                            sub_task_id: "sub-1".to_string(),
                            status: Status::Success as i32,
                            ..Default::default()
                        }),
                    }),
                    ..Default::default()
                }],
                ..Default::default()
            }
        );
    }
}
