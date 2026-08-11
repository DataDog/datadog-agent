use anyhow::{Context, Result};
use core::*;
use serde_json::Value;

use crate::config::{CheckConfig, SubTask};
use crate::payload::{Match, ScanEventPayload};

/// Check implementation (scaffolding).
pub fn check(check: &AgentCheck) -> Result<()> {
    let config = CheckConfig::from_instance(check)?;
    println!(
        "datasecurity: check started (task_id={}, {} sub task(s))",
        config.task_id,
        config.scan_data.len()
    );

    for sub_task in &config.scan_data {
        run_sub_task(check, &config, sub_task)?;
    }

    println!("datasecurity: check completed");
    Ok(())
}

fn run_sub_task(check: &AgentCheck, config: &CheckConfig, sub_task: &SubTask) -> Result<()> {
    println!(
        "datasecurity: running sub task (sub_task_id={})",
        sub_task.sub_task_id
    );

    // TODO(DSEC-139): fetch the rows from postgres.
    let data = fetch_data(sub_task);
    // TODO(DSEC-138): scan the rows with the SDS scanner.
    let matches = scan(&data);

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
    let payload_json = serde_json::to_string(&payload).context("serializing scan event payload")?;
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

/// Placeholder scan over the returned columns.
// TODO(DSEC-138): replace with the SDS scanner.
fn scan(scan_result: &Value) -> Vec<Match> {
    let mut matches = Vec::new();
    if let Some(emails) = scan_result.get("email").and_then(Value::as_array) {
        let count = emails
            .iter()
            .filter(|value| value.as_str().is_some_and(|email| email.contains('@')))
            .count() as i64;
        if count > 0 {
            matches.push(Match {
                rule_id: "email-scanner".to_string(),
                column_name: "email".to_string(),
                count_matched_rows: count,
            });
        }
    }
    matches
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn scan_placeholder_response_returns_email_matches() {
        let scan_result = json!({
            "email": ["alice@example.com", "bob@test.com", "charlie@corp.com"],
        });
        let matches = scan(&scan_result);
        assert_eq!(matches.len(), 1);
        assert_eq!(matches[0].column_name, "email");
        assert_eq!(matches[0].count_matched_rows, 3);
    }
}
