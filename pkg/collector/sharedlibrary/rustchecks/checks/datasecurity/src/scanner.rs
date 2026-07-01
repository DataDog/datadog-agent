use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::{Context, Result};
use dd_sds::{
    CreateScannerError, RegexRuleConfig, RootRuleConfig, RuleConfig, RuleMatch, Scanner,
    ScannerBuilder,
};
use serde_json::{Map, Value};

use crate::payload::{ScanningRule, TableMatch};

pub struct ScannerHandle {
    scanner: Scanner,
    rule_ids: Vec<String>,
}

impl ScannerHandle {
    pub fn new(rules: &[ScanningRule]) -> Result<Self, CreateScannerError> {
        let mut rule_ids = Vec::with_capacity(rules.len());
        let compiled: Vec<RootRuleConfig<Arc<dyn RuleConfig>>> = rules
            .iter()
            .map(|rule| {
                rule_ids.push(rule.id.clone());
                RootRuleConfig::new(RegexRuleConfig::new(&rule.regex).build())
            })
            .collect();

        let scanner = ScannerBuilder::new(&compiled).build()?;
        Ok(Self { scanner, rule_ids })
    }

    pub fn scan_columns(
        &self,
        columns: &HashMap<String, Vec<Value>>,
    ) -> Result<Vec<TableMatch>> {
        scan_columns(&self.scanner, &self.rule_ids, columns)
    }
}

/// Scans column-oriented query results with a single `scan` call and returns
/// one `TableMatch` per (column, rule) pair, mirroring Go `scanColumns`.
pub fn scan_columns(
    scanner: &Scanner,
    rule_ids: &[String],
    columns: &HashMap<String, Vec<Value>>,
) -> Result<Vec<TableMatch>> {
    let mut event = Map::new();
    for (name, values) in columns {
        let str_values: Vec<Value> = values
            .iter()
            .filter_map(|v| v.as_str().map(|s| Value::String(s.to_string())))
            .collect();
        event.insert(name.clone(), Value::Array(str_values));
    }

    let mut event_value = Value::Object(event);
    let hits = scanner
        .scan(&mut event_value)
        .context("scanning query result")?;

    aggregate_table_matches(rule_ids, &hits)
}

fn aggregate_table_matches(rule_ids: &[String], hits: &[RuleMatch]) -> Result<Vec<TableMatch>> {
    let mut rows: HashMap<(String, String), HashSet<String>> = HashMap::new();

    for hit in hits {
        let path = hit.path.to_string();
        let column = column_name_from_path(&path);
        let rule_id = rule_ids
            .get(hit.rule_index)
            .cloned()
            .unwrap_or_default();
        rows.entry((column, rule_id))
            .or_default()
            .insert(path);
    }

    let mut keys: Vec<(String, String)> = rows.keys().cloned().collect();
    keys.sort_by(|a, b| a.0.cmp(&b.0).then_with(|| a.1.cmp(&b.1)));

    Ok(keys
        .into_iter()
        .map(|(column, rule_id)| TableMatch {
            rule_id: rule_id.clone(),
            column_name: column.clone(),
            count_matched_rows: rows[&(column, rule_id)].len() as i64,
        })
        .collect())
}

fn column_name_from_path(path: &str) -> String {
    if let Some(i) = path.find(['[', '.']) {
        path[..i].to_string()
    } else {
        path.to_string()
    }
}
