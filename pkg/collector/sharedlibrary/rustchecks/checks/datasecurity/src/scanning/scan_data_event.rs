//! SDS [`Event`] for [`ScanData`]: one table of columns and rows.

use anyhow::{Context, Result};
use dd_sds::{Event, EventVisitor, Path, PathSegment, ScannerError, Utf8Encoding};

use crate::backend::ScanData;

/// Path is `[Index(row), Index(column)]`, matching `visit_event`.
impl Event for ScanData {
    type Encoding = Utf8Encoding;

    fn visit_event<'a>(
        &'a mut self,
        visitor: &mut impl EventVisitor<'a>,
    ) -> Result<(), ScannerError> {
        for (row_idx, row) in self.rows.iter().enumerate() {
            visitor.push_segment(row_idx.into());
            for (col_idx, cell) in row.iter().enumerate() {
                let Some(value) = cell.as_deref() else {
                    continue;
                };
                visitor.push_segment(col_idx.into());
                visitor.visit_string(value)?;
                visitor.pop_segment();
            }
            visitor.pop_segment();
        }
        Ok(())
    }

    fn visit_string_mut(&mut self, path: &Path<'_>, visit: impl FnOnce(&mut String) -> bool) {
        let Some((row, col)) = path_row_col(path) else {
            return;
        };
        if let Some(Some(value)) = self.rows.get_mut(row).and_then(|r| r.get_mut(col)) {
            visit(value);
        }
    }
}

impl ScanData {
    pub(super) fn column_name(&self, path: &Path<'_>) -> Result<&str> {
        let (_, col) =
            path_row_col(path).with_context(|| format!("unexpected sds path {path:?}"))?;
        self.scanned_columns
            .get(col)
            .map(|c| c.name.as_str())
            .with_context(|| format!("sds path column index {col} is out of range"))
    }
}

fn path_row_col(path: &Path<'_>) -> Option<(usize, usize)> {
    match path.segments.as_slice() {
        [PathSegment::Index(row), PathSegment::Index(col), ..] => Some((*row, *col)),
        _ => None,
    }
}
