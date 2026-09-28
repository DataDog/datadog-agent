//! SDS [`Event`] for [`ScanData`]: one table of columns and rows.

use dd_sds::{Event, EventVisitor, Path, PathSegment, ScannerError, Utf8Encoding};

use crate::backend::ScanData;

/// Path is `[Index(row), Index(column), Field(column name)]`, matching `visit_event`.
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
                let Some(column) = self.scanned_columns.get(col_idx) else {
                    continue;
                };
                // Index(col) lets visit_string_mut address the cell without looking up
                // the column name on every match.
                visitor.push_segment(col_idx.into());
                visitor.push_segment(column.name.as_str().into());
                visitor.visit_string(value)?;
                visitor.pop_segment();
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

fn path_row_col(path: &Path<'_>) -> Option<(usize, usize)> {
    match path.segments.as_slice() {
        [
            PathSegment::Index(row),
            PathSegment::Index(col),
            PathSegment::Field(_),
            ..,
        ] => Some((*row, *col)),
        _ => None,
    }
}
