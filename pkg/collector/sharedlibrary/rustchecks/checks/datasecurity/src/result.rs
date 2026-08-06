use crate::backend::ScannedColumn;
use crate::config::{CheckConfig, SubTask};
use crate::proto::{
    self, PostgresScannedColumn, ScanMetadata, ScanResult, ScanTaskMetadata, SdsResultPayload,
    Status, TableMatch,
};

/// The result of scanning one sub task: the matches plus the statistics
/// reported in the result's `PostgresTable` location.
#[derive(Debug, Default)]
pub(crate) struct ScanOutcome {
    /// One entry per `(column, rule)` that matched.
    pub matches: Vec<TableMatch>,
    /// The columns that were scanned (name + source data type).
    pub scanned_columns: Vec<ScannedColumn>,
    /// Number of rows returned by the query and scanned.
    pub scanned_row_count: i64,
}

/// Builds the `SdsResultPayload` protobuf for one sub task.
pub(crate) fn build_sds_result(
    config: &CheckConfig,
    sub_task: &SubTask,
    status: Status,
    failure_reason: &str,
    outcome: ScanOutcome,
) -> SdsResultPayload {
    let entity = &sub_task.entity;

    let location = proto::ScanLocation {
        scan_location: Some(proto::scan_location::ScanLocation::PostgresTable(
            proto::PostgresTable {
                database_cluster_name: entity.database_cluster_name.clone(),
                database_instance_name: entity.database_instance_name.clone(),
                database_host_name: sub_task.connection.host.clone(),
                database_name: entity.database.clone(),
                schema_name: entity.schema.clone(),
                table_name: entity.table.clone(),
                scanned_row_count: outcome.scanned_row_count,
                scanned_columns: outcome
                    .scanned_columns
                    .into_iter()
                    .map(|column| PostgresScannedColumn {
                        name: column.name,
                        data_type: column.data_type,
                    })
                    .collect(),
                // TODO(DSEC-227): populate table_row_count if possible.
                ..Default::default()
            },
        )),
        ..Default::default()
    };

    // TODO(DSEC-180): populate duration, started_at and ended_at.
    let scan_result = ScanResult {
        table_matches: outcome.matches,
        location: Some(location),
        scan_metadata: Some(ScanMetadata {
            scan_task_metadata: Some(ScanTaskMetadata {
                task_id: config.task_id.clone(),
                sub_task_id: sub_task.sub_task_id.clone(),
                status: status as i32,
                failure_reason: (!failure_reason.is_empty()).then(|| failure_reason.to_string()),
                ..Default::default()
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
        // The scanning source is the Agent. TODO(DSEC-228): populate hostname and
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

/// Current Unix time in milliseconds, clamped to zero if the system clock is
/// set before the Unix epoch.
fn now_unix_millis() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};

    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

/// Resource name (`<instance_name>.<database>.<schema>.<table>`).
fn resource_name(sub_task: &SubTask) -> String {
    let entity = &sub_task.entity;
    format!(
        "{}.{}.{}.{}",
        entity.database_instance_name, entity.database, entity.schema, entity.table
    )
}
