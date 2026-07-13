use anyhow::{Context, Result};
use shlib_core::*;
use std::time::{SystemTime, UNIX_EPOCH};

use crate::backend::{self, InstanceConnections, QueryResult};
use crate::constants::{AGENTLESS_REGION, AGENTLESS_VERSION};
use crate::payload::{
    Agentless, PostgresConnection, Resource, ScanConfig, ScanResult, ScanningSource,
    SdsResultPayload,
};
use crate::scanner::ScannerHandle;

/// Check implementation.
pub fn check(check: &AgentCheck) -> Result<()> {
    let task_id: Option<String> = check.instance.get("task_id").ok();
    let scan_config: ScanConfig = check
        .instance
        .get("scan_config")
        .context("reading scan_config from instance config")?;
    let backend: String = check
        .instance
        .get("backend")
        .context("reading backend from instance config")?;
    let postgres: Option<PostgresConnection> = check.instance.get("postgres").ok();

    println!(
        "datasecurity: check started (task_id={}, backend={backend})",
        task_id.as_deref().unwrap_or("<none>")
    );

    let task = scan_config
        .tasks
        .first()
        .context("scan config has no tasks")?;

    if task.scanning_rules.is_empty() {
        anyhow::bail!("no scanning rules in payload");
    }

    let scanner_handle = ScannerHandle::new(&task.scanning_rules)
        .context("creating sds scanner")?;
    println!(
        "datasecurity: sds scanner created with {} rule(s)",
        task.scanning_rules.len()
    );

    let connections = InstanceConnections {
        postgres: postgres.as_ref(),
    };
    backend::log_connection(&backend, &connections)?;
    let query_result =
        backend::execute_scan(&backend, task, &connections).context("running backend scan")?;

    log_scan_stats(&backend, &query_result);

    let payload = build_sds_result_payload(&backend, &scanner_handle, query_result)
        .context("building sds result payload")?;

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

fn log_scan_stats(backend: &str, query_result: &QueryResult) {
    let row_count = query_result
        .columns
        .values()
        .map(|values| values.len())
        .max()
        .unwrap_or(0);
    if let Ok(columns_json) = serde_json::to_string(&query_result.columns) {
        println!(
            "datasecurity: {backend} scan returned {row_count} row(s) in {:.3}s: {columns_json}",
            query_result.duration_s
        );
    } else {
        println!(
            "datasecurity: {backend} scan returned {row_count} row(s) in {:.3}s",
            query_result.duration_s
        );
    }
}

fn build_sds_result_payload(
    backend: &str,
    scanner_handle: &ScannerHandle,
    result: QueryResult,
) -> Result<SdsResultPayload> {
    let table_matches = scanner_handle.scan_columns(&result.columns)?;

    let resource_name = if result.host.is_empty() {
        result.database.clone()
    } else {
        result.host.clone()
    };

    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .context("system time before unix epoch")?
        .as_millis() as i64;

    let (resource_type, location) = backend::build_location(backend, &result)?;

    Ok(SdsResultPayload {
        scan_source: "AGENTLESS",
        timestamp,
        resource: Resource {
            resource_type: resource_type.to_string(),
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
            location,
        }],
    })
}
