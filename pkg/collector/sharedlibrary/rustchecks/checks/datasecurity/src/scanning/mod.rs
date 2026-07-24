//! SDS scanning: parse rules and run the dd-sds scanner over query results.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::{Context, Result};
use dd_sds::{
    Path, PathSegment, RootRuleConfig, RuleConfig, RuleMatch, Scanner as SdsScanner, ScannerBuilder,
};
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
            .context("failed to build sds scanner")?;
        Ok(Self { scanner, rule_ids })
    }

    /// Scans `{ column: [values] }` and returns one `Match` per (column, rule).
    pub fn scan(&self, data: Value) -> Result<Vec<Match>> {
        let mut event = data;
        let hits = self
            .scanner
            .scan(&mut event)
            .context("failed to scan query result")?;

        aggregate_matches(&self.rule_ids, &hits)
    }
}

/// Groups hits into `(column, rule)` pairs and counts matched rows.
fn aggregate_matches(rule_ids: &[String], hits: &[RuleMatch]) -> Result<Vec<Match>> {
    let mut rows: HashMap<(&str, usize), HashSet<&Path>> = HashMap::new();
    // Bucket each hit by (column, rule), collecting its distinct row paths.
    for hit in hits {
        rows.entry((column_name_from_path(&hit.path), hit.rule_index))
            .or_default()
            .insert(&hit.path);
    }

    // Convert to matches.
    rows.into_iter()
        .map(|((column, rule_index), paths)| {
            // return an error if the rule index is unknown.
            let rule_id = rule_ids
                .get(rule_index)
                .cloned()
                .with_context(|| format!("scanner returned unknown rule index {rule_index}"))?;
            Ok(Match {
                rule_id,
                column_name: column.to_string(),
                count_matched_rows: paths.len() as i64,
            })
        })
        .collect()
}

/// Column name = the path's leading field segment.
/// - `[Field("email"), Index(3)]` -> `"email"`
/// - `[Field("foo[bar]"), Index(0)]` -> `"foo[bar]"`
fn column_name_from_path<'p>(path: &'p Path<'_>) -> &'p str {
    match path.segments.first() {
        Some(PathSegment::Field(field)) => field.as_ref(),
        _ => "",
    }
}
