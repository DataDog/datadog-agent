//! SDS scanning: parse rules and run the dd-sds scanner over query results.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::{Context, Result};
use dd_sds::{
    Path, PathSegment, RootRuleConfig, RuleConfig, RuleMatch, Scanner as SdsScanner, ScannerBuilder,
};
use serde_json::Value;

use crate::proto::TableMatch as Match;

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

/// Frees dd-sds' global regex caches. Call after the `Scanner` is dropped, as
/// regexes referenced by a live scanner are not reclaimed.
pub fn clear_caches() {
    dd_sds::clear_all_caches();
}

/// Returns freed native heap pages to Linux.
pub fn malloc_trim() {
    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    unsafe {
        libc::malloc_trim(0);
    }
}

/// Groups hits into `(column, rule)` pairs and counts matched rows and total
/// matches.
fn aggregate_matches(rule_ids: &[String], hits: &[RuleMatch]) -> Result<Vec<Match>> {
    // For each (column, rule): the distinct matched row paths and the total
    // number of matches (a single row may contain several matches).
    let mut buckets: HashMap<(&str, usize), (HashSet<&Path>, i64)> = HashMap::new();
    for hit in hits {
        let (paths, count_matches) = buckets
            .entry((column_name_from_path(&hit.path), hit.rule_index))
            .or_default();
        paths.insert(&hit.path);
        *count_matches += 1;
    }

    // Convert to matches.
    buckets
        .into_iter()
        .map(|((column, rule_index), (paths, count_matches))| {
            // return an error if the rule index is unknown.
            let rule_id = rule_ids
                .get(rule_index)
                .cloned()
                .with_context(|| format!("scanner returned unknown rule index {rule_index}"))?;
            Ok(Match {
                rule_id,
                column_name: column.to_string(),
                count_matched_rows: paths.len() as i64,
                count_matches,
                ..Default::default()
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
