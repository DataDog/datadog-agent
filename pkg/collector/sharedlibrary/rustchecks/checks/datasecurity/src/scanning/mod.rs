//! Self-contained SDS scanning: rule config parsing (`rule`) plus building and
//! running the dd-sds scanner over column-oriented query results. Everything
//! scanning-related lives in this module so the rest of the check stays lean.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::{Context, Result};
use dd_sds::{RootRuleConfig, RuleConfig, RuleMatch, Scanner as SdsScanner, ScannerBuilder};
use serde_json::{Map, Value};

use crate::payload::Match;

mod rule;
pub use rule::ScanningRule;

#[cfg(test)]
mod tests;

/// A built dd-sds scanner plus the rule ids in builder order. dd-sds reports
/// matches by rule index, so we keep the ids to map matches back to our rules.
pub struct Scanner {
    scanner: SdsScanner,
    rule_ids: Vec<String>,
}

impl Scanner {
    /// Builds a scanner from the check's scanning rules. The full dd-sds rule
    /// surface (pattern, proximity keywords, suppressions, precedence, secondary
    /// validators such as Luhn/JWT, ...) is supported because each rule carries
    /// the flattened `RootRuleConfig` verbatim.
    pub fn new(rules: &[ScanningRule]) -> Result<Self> {
        let mut rule_ids = Vec::with_capacity(rules.len());
        let compiled: Vec<RootRuleConfig<Arc<dyn RuleConfig>>> = rules
            .iter()
            .map(|rule| {
                rule_ids.push(rule.id.clone());
                rule.config.clone().into_dyn()
            })
            .collect();

        let scanner = ScannerBuilder::new(&compiled)
            .build()
            .context("building sds scanner")?;
        Ok(Self { scanner, rule_ids })
    }

    /// Scans a column-oriented result (`{ column: [values...] }`) and returns
    /// one `Match` per (column, rule) pair with the count of matched rows.
    pub fn scan(&self, data: &Value) -> Result<Vec<Match>> {
        let mut event = Map::new();
        if let Some(columns) = data.as_object() {
            for (name, values) in columns {
                let str_values: Vec<Value> = values
                    .as_array()
                    .map(|vs| {
                        vs.iter()
                            .filter_map(|v| v.as_str().map(|s| Value::String(s.to_string())))
                            .collect()
                    })
                    .unwrap_or_default();
                event.insert(name.clone(), Value::Array(str_values));
            }
        }

        let mut event_value = Value::Object(event);
        let hits = self
            .scanner
            .scan(&mut event_value)
            .context("scanning query result")?;

        Ok(aggregate_matches(&self.rule_ids, &hits))
    }
}

/// Groups raw dd-sds hits into `(column, rule)` pairs, counting distinct matched
/// rows (paths) per pair. Output is sorted so emitted events are deterministic.
fn aggregate_matches(rule_ids: &[String], hits: &[RuleMatch]) -> Vec<Match> {
    let mut rows: HashMap<(String, String), HashSet<String>> = HashMap::new();
    for hit in hits {
        let path = hit.path.to_string();
        let column = column_name_from_path(&path);
        let rule_id = rule_ids.get(hit.rule_index).cloned().unwrap_or_default();
        rows.entry((column, rule_id)).or_default().insert(path);
    }

    let mut matches: Vec<Match> = rows
        .into_iter()
        .map(|((column, rule_id), paths)| Match {
            rule_id,
            column_name: column,
            count_matched_rows: paths.len() as i64,
        })
        .collect();

    matches.sort_by(|a, b| {
        a.column_name
            .cmp(&b.column_name)
            .then_with(|| a.rule_id.cmp(&b.rule_id))
    });
    matches
}

/// `foo[0]` / `foo.bar` -> `foo`.
fn column_name_from_path(path: &str) -> String {
    match path.find(['[', '.']) {
        Some(i) => path[..i].to_string(),
        None => path.to_string(),
    }
}
