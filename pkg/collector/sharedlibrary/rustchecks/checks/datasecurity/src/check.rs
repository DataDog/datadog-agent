use anyhow::{Context, Result};
use shlib_core::*;
use std::time::{SystemTime, UNIX_EPOCH};

use crate::constants::{
    AGENTLESS_REGION, AGENTLESS_VERSION, RESOURCE_TYPE_RDS_INSTANCE,
};
use crate::payload::{
    Agentless, PostgresConnection, Resource, ScanConfig, ScanLocation, ScanResult, ScanningSource,
    SdsResultPayload,
};
use crate::postgres::{run_postgres_query, QueryResult};
use crate::scanner::ScannerHandle;

/// Check implementation.
pub fn check(check: &AgentCheck) -> Result<()> {
    let task_id: Option<String> = check.instance.get("task_id").ok();
    let scan_config: ScanConfig = check
        .instance
        .get("scan_config")
        .context("reading scan_config from instance config")?;
    let postgres: PostgresConnection = check
        .instance
        .get("postgres")
        .context("reading postgres connection from instance config")?;

    println!(
        "datasecurity: check started (task_id={})",
        task_id.as_deref().unwrap_or("<none>")
    );

    let task = scan_config
        .tasks
        .first()
        .context("scan config has no tasks")?;

    if task.scanning_rules.is_empty() {
        anyhow::bail!("no scanning rules in payload");
    }

    let postgres_scan = task
        .scan_data
        .postgres
        .as_ref()
        .context("no postgres scan_data in payload")?;

    let query = &postgres_scan.query;
    let table = &postgres_scan.table;

    println!(
        "datasecurity: loaded scan config (rules={}, table={}, query={query})",
        task.scanning_rules.len(),
        table,
    );

    let scanner_handle = ScannerHandle::new(&task.scanning_rules)
        .context("creating sds scanner")?;
    println!(
        "datasecurity: sds scanner created with {} rule(s)",
        task.scanning_rules.len()
    );

    println!(
        "datasecurity: connecting to postgres host={} port={} dbname={} user={}",
        postgres.host, postgres.port, postgres.dbname, postgres.username
    );
    println!("datasecurity: running postgres query: {query}");

    let query_result =
        run_postgres_query(&postgres, query).context("running postgres query")?;

    let row_count = query_result
        .columns
        .values()
        .map(|values| values.len())
        .max()
        .unwrap_or(0);
    if let Ok(columns_json) = serde_json::to_string(&query_result.columns) {
        println!(
            "datasecurity: postgres query returned {row_count} row(s) in {:.3}s: {columns_json}",
            query_result.duration_s
        );
    } else {
        println!(
            "datasecurity: postgres query returned {row_count} row(s) in {:.3}s",
            query_result.duration_s
        );
    }

    let payload = build_sds_result_payload(&scanner_handle, query_result, table)
        .context("building sds result payload")?;

    // Scanner is dropped here when `scanner_handle` goes out of scope.
    drop(scanner_handle);

    println!(
        "datasecurity: built sds result payload ({} table match(es))",
        payload.scan_results[0].table_matches.len()
    );

    let payload_bytes = crate::proto::encode(&payload);
    println!(
        "datasecurity: marshalled sds result protobuf ({} byte(s))",
        payload_bytes.len()
    );

    check.event_platform_event_bytes(&payload_bytes, crate::constants::SDS_RESULT_EVENT_TYPE)?;

    let payload_json =
        serde_json::to_string(&payload).context("serializing sds result payload for event")?;

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

    println!("datasecurity: check completed");
    Ok(())
}
fn build_sds_result_payload(
    scanner_handle: &ScannerHandle,
    result: QueryResult,
    table_name: &str,
) -> Result<SdsResultPayload> {
    let table_matches = scanner_handle.scan_columns(&result.columns)?;

    let resource_name = if result.host.is_empty() {
        result.dbname.clone()
    } else {
        result.host.clone()
    };

    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .context("system time before unix epoch")?
        .as_millis() as i64;

    Ok(SdsResultPayload {
        scan_source: "AGENTLESS",
        timestamp,
        resource: Resource {
            resource_type: RESOURCE_TYPE_RDS_INSTANCE.to_string(),
            name: resource_name.clone(),
        },
        scanning_source: ScanningSource {
            agentless: Agentless {
                version: AGENTLESS_VERSION.to_string(),
                region: AGENTLESS_REGION.to_string(),
            },
        },
        scan_results: vec![ScanResult {
            duration: (result.duration_s * 1000.0) as i64,
            table_matches,
            location: ScanLocation {
                database: result.dbname.clone(),
                rds_table: crate::payload::RdsTable {
                    instance_arn: result.host.clone(),
                    database_name: result.dbname.clone(),
                    table_name: table_name.to_string(),
                },
            },
        }],
    })
}

