use anyhow::{Context, Result};
use core::*;
use serde_json::Value;

use crate::config::{CheckConfig, SubTask};
use crate::payload::ScanEventPayload;
use crate::scanning::Scanner;

/// Check implementation (scaffolding).
pub fn check(check: &AgentCheck) -> Result<()> {
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
        "datasecurity: running sub task (sub_task_id={})",
        sub_task.sub_task_id
    );

    // TODO(DSEC-139): fetch the rows from postgres.
    let data = fetch_data(sub_task);
    let matches = scanner.scan(&data).context("failed to scan sub task data")?;

    let payload = ScanEventPayload {
        task_id: config.task_id.clone(),
        sub_task_id: sub_task.sub_task_id.clone(),
        matches,
    };

    println!(
        "datasecurity: built scaffold event payload ({} match(es))",
        payload.matches.len()
    );

    // TODO(DSEC-140): send sdsresult rather than an event
    let payload_json = serde_json::to_string(&payload).context("failed to serialize scan event payload")?;
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

/// Mimics a postgres fetch by returning the sub task's placeholder response.
// TODO(DSEC-139): replace with a real postgres query.
fn fetch_data(sub_task: &SubTask) -> Value {
    sub_task.placeholder_response.clone()
}
