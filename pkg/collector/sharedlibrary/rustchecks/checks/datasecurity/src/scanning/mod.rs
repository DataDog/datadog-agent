//! SDS scanning: parse rules and run the dd-sds scanner over query results.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::{Context, Result};
use dd_sds::{RootRuleConfig, RuleConfig, RuleMatch, Scanner as SdsScanner, ScannerBuilder};
use serde_json::Value;

use crate::payload::Match;

mod rule;
pub use rule::ScanningRule;

#[cfg(test)]
mod tests;

/// A dd-sds scanner plus the rule ids, used to map matches back to rules.
pub struct Scanner {
    scanner: SdsScanner,
    rule_ids: Vec<String>,
}

impl Scanner {
    /// Builds a scanner from the check's scanning rules.
    pub fn new(rules: &[ScanningRule]) -> Result<Self> {
        let mut rule_ids = Vec::with_capacity(rules.len());
        let scanner_rules: Vec<RootRuleConfig<Arc<dyn RuleConfig>>> = rules
            .iter()
            .map(|rule| {
                rule_ids.push(rule.id.clone());
                rule.config.clone().into_dyn()
            })
            .collect();

        let scanner = ScannerBuilder::new(&scanner_rules)
            .build()
            .context("building sds scanner")?;
        Ok(Self { scanner, rule_ids })
    }

    /// Scans `{ column: [values] }` and returns one `Match` per (column, rule).
    pub fn scan(&self, data: &Value) -> Result<Vec<Match>> {
        let mut event = data.clone();
        let hits = self
            .scanner
            .scan(&mut event)
            .context("scanning query result")?;

        Ok(aggregate_matches(&self.rule_ids, &hits))
    }
}

/// Groups hits into `(column, rule)` pairs and counts matched rows. Sorted for
/// deterministic output.
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

/// `foo[0]` -> `foo`.
fn column_name_from_path(path: &str) -> String {
    match path.find('[') {
        Some(i) => path[..i].to_string(),
        None => path.to_string(),
    }
}
