//! SDS scanning: parse rules and run the dd-sds scanner over query rows.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::{Context, Result};
use dd_sds::{
    Event, EventVisitor, Path, PathSegment, RootRuleConfig, RuleConfig, RuleMatch,
    Scanner as SdsScanner, ScannerBuilder, ScannerError, Utf8Encoding,
};

use crate::backend::{ScanCells, ScannedColumn};
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

/// Accumulates per-row SDS hits while the backend streams rows.
pub(crate) struct ScanFeed<'a> {
    scanner: &'a Scanner,
    buckets: HashMap<(String, usize), (i64, i64)>,
    row_count: i64,
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

    pub(crate) fn feed(&self) -> ScanFeed<'_> {
        ScanFeed {
            scanner: self,
            buckets: HashMap::new(),
            row_count: 0,
        }
    }

    /// Scans each row as an SDS [`Event`] and returns one `Match` per (column, rule)
    /// plus the number of rows consumed.
    pub fn scan(
        &self,
        columns: &[ScannedColumn],
        rows: impl Iterator<Item = Result<impl ScanCells>>,
    ) -> Result<(Vec<Match>, i64)> {
        let mut feed = self.feed();
        for row in rows {
            let row = row?;
            feed.push(columns, &row)?;
        }
        feed.finish()
    }
}

impl ScanFeed<'_> {
    /// Scans one row via SDS [`Event`] visit; cells are not collected first.
    pub fn push(&mut self, columns: &[ScannedColumn], row: &dyn ScanCells) -> Result<()> {
        self.row_count += 1;
        let mut event = CellsEvent {
            columns,
            row,
            buf: String::new(),
        };
        let hits = self
            .scanner
            .scanner
            .scan(&mut event)
            .context("failed to scan query result")?;
        add_row_hits(&mut self.buckets, &hits);
        Ok(())
    }

    pub fn finish(self) -> Result<(Vec<Match>, i64)> {
        Ok((
            matches_from_buckets(&self.scanner.rule_ids, self.buckets)?,
            self.row_count,
        ))
    }
}

/// One query row as an SDS [`Event`]. Strings are visited in place; NULL cells are skipped.
struct CellsEvent<'a> {
    columns: &'a [ScannedColumn],
    row: &'a dyn ScanCells,
    buf: String,
}

impl Event for CellsEvent<'_> {
    type Encoding = Utf8Encoding;

    fn visit_event<'a>(
        &'a mut self,
        visitor: &mut impl EventVisitor<'a>,
    ) -> Result<(), ScannerError> {
        let columns = self.columns;
        let row = self.row;
        for (index, column) in columns.iter().enumerate() {
            let Some(value) = row.cell(index) else {
                continue;
            };
            visitor.push_segment(column.name.as_str().into());
            visitor.visit_string(value)?;
            visitor.pop_segment();
        }
        Ok(())
    }

    fn visit_string_mut(&mut self, path: &Path<'_>, visit: impl FnOnce(&mut String) -> bool) {
        let Some(PathSegment::Field(field)) = path.segments.first() else {
            return;
        };
        let Some(index) = self.columns.iter().position(|column| column.name == *field) else {
            return;
        };
        if let Some(value) = self.row.cell(index) {
            self.buf.clear();
            self.buf.push_str(value);
            visit(&mut self.buf);
        }
    }
}

fn add_row_hits(buckets: &mut HashMap<(String, usize), (i64, i64)>, hits: &[RuleMatch]) {
    let mut seen: HashSet<(&str, usize)> = HashSet::new();
    for hit in hits {
        let column = column_name_from_path(&hit.path);
        let first = seen.insert((column, hit.rule_index));
        let (matched_rows, count_matches) = buckets
            .entry((column.to_string(), hit.rule_index))
            .or_default();
        if first {
            *matched_rows += 1;
        }
        *count_matches += 1;
    }
}

fn matches_from_buckets(
    rule_ids: &[String],
    buckets: HashMap<(String, usize), (i64, i64)>,
) -> Result<Vec<Match>> {
    buckets
        .into_iter()
        .map(
            |((column_name, rule_index), (count_matched_rows, count_matches))| {
                let rule_id = rule_ids
                    .get(rule_index)
                    .cloned()
                    .with_context(|| format!("scanner returned unknown rule index {rule_index}"))?;
                Ok(Match {
                    rule_id,
                    column_name,
                    count_matched_rows,
                    count_matches,
                    ..Default::default()
                })
            },
        )
        .collect()
}

/// Column name = the path's leading field segment.
/// - `[Field("email")]` -> `"email"`
/// - `[Field("foo[bar]")]` -> `"foo[bar]"`
fn column_name_from_path<'p>(path: &'p Path<'_>) -> &'p str {
    match path.segments.first() {
        Some(PathSegment::Field(field)) => field.as_ref(),
        _ => "",
    }
}
