use crate::backend::ScannedColumn;
use crate::config::{CheckConfig, SubTask};
use crate::proto::{
    self, MysqlScannedColumn, PostgresScannedColumn, ScanMetadata, ScanResult, ScanTaskMetadata,
    SdsResultPayload, Status, TableMatch,
};

/// The result of scanning one sub task: the matches plus the statistics
/// reported in the result's database table location.
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
    let ScanOutcome {
        matches,
        scanned_columns,
        scanned_row_count,
    } = outcome;
    let database_host_name = if entity.database_host_name.is_empty() {
        sub_task.connection.host.clone()
    } else {
        entity.database_host_name.clone()
    };

    let location = proto::ScanLocation {
        scan_location: Some(match entity.platform.as_str() {
            "mysql" => proto::scan_location::ScanLocation::MysqlTable(proto::MysqlTable {
                database_cluster_name: entity.database_cluster_name.clone(),
                database_instance_name: entity.database_instance_name.clone(),
                database_host_name: database_host_name.clone(),
                database_name: entity.database.clone(),
                schema_name: entity.schema.clone(),
                table_name: entity.table.clone(),
                scanned_row_count,
                scanned_columns: scanned_columns
                    .iter()
                    .map(|column| MysqlScannedColumn {
                        name: column.name.clone(),
                        data_type: column.data_type.clone(),
                    })
                    .collect(),
                // TODO(DSEC-227): populate table_row_count if possible.
                ..Default::default()
            }),
            _ => proto::scan_location::ScanLocation::PostgresTable(proto::PostgresTable {
                database_cluster_name: entity.database_cluster_name.clone(),
                database_instance_name: entity.database_instance_name.clone(),
                database_host_name,
                database_name: entity.database.clone(),
                schema_name: entity.schema.clone(),
                table_name: entity.table.clone(),
                scanned_row_count,
                scanned_columns: scanned_columns
                    .into_iter()
                    .map(|column| PostgresScannedColumn {
                        name: column.name,
                        data_type: column.data_type,
                    })
                    .collect(),
                // TODO(DSEC-227): populate table_row_count if possible.
                ..Default::default()
            }),
        }),
        ..Default::default()
    };

    // TODO(DSEC-180): populate duration, started_at and ended_at.
    let scan_result = ScanResult {
        table_matches: matches,
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
            r#type: match entity.platform.as_str() {
                "mysql" => "mysql_table",
                _ => "postgres_table",
            }
            .to_string(),
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

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::{ScanOutcome, build_sds_result};
    use crate::config::{CheckConfig, Connection, Entity, SubTask};
    use crate::proto::{Status, scan_location};

    #[test]
    fn builds_mysql_table_location() {
        let config = CheckConfig {
            task_id: "task".to_string(),
            scanning_rules: vec![],
            scan_data: vec![],
        };
        let sub_task = SubTask {
            sub_task_id: "subtask".to_string(),
            connection: Connection {
                host: "mysql.example.com".to_string(),
                ..Default::default()
            },
            entity: Entity {
                platform: "mysql".to_string(),
                database_cluster_name: "cluster".to_string(),
                database_instance_name: "instance".to_string(),
                database_host_name: "mysql.example.com".to_string(),
                database: "app".to_string(),
                schema: "app".to_string(),
                table: "users".to_string(),
            },
            query: "SELECT name FROM users".to_string(),
            timeout: Duration::from_secs(30),
        };

        let payload = build_sds_result(
            &config,
            &sub_task,
            Status::Success,
            "",
            ScanOutcome::default(),
        );

        assert_eq!(payload.resource.unwrap().r#type, "mysql_table");
        assert!(matches!(
            payload.scan_results[0]
                .location
                .as_ref()
                .and_then(|location| location.scan_location.as_ref()),
            Some(scan_location::ScanLocation::MysqlTable(_))
        ));
    }
}
