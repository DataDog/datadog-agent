//! SDS scanning: parse rules and run the dd-sds scanner over query rows.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::{Context, Result};
use dd_sds::{Path, RootRuleConfig, RuleConfig, RuleMatch, Scanner as SdsScanner, ScannerBuilder};

use crate::backend::ScanData;
use crate::proto::TableMatch as Match;

mod rule;
mod scan_data_event;
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

    /// Scans `data` as one SDS [`Event`] and returns one `Match` per (column, rule).
    pub fn scan(&self, data: &mut ScanData) -> Result<Vec<Match>> {
        let hits = self
            .scanner
            .scan(data)
            .context("failed to scan query result")?;

        aggregate_matches(&self.rule_ids, data, &hits)
    }
}

fn aggregate_matches(
    rule_ids: &[String],
    data: &ScanData,
    hits: &[RuleMatch],
) -> Result<Vec<Match>> {
    let mut buckets: HashMap<(&str, usize), (HashSet<&Path>, i64)> = HashMap::new();
    for hit in hits {
        let column = data.column_name(&hit.path)?;
        let (paths, count_matches) = buckets.entry((column, hit.rule_index)).or_default();
        paths.insert(&hit.path);
        *count_matches += 1;
    }

    buckets
        .into_iter()
        .map(|((column, rule_index), (paths, count_matches))| {
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
