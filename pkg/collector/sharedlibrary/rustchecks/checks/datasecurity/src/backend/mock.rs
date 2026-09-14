//! Test-only scan engine (`platform: mock`) yielding caller-controlled rows.

use std::cell::RefCell;

use anyhow::Result;

use super::{ScanEngine, ScanFn, ScanRow, ScannedColumn};
use crate::config::SubTask;

thread_local! {
    static DATA: RefCell<(Vec<ScannedColumn>, Vec<ScanRow>)> =
        const { RefCell::new((Vec::new(), Vec::new())) };
}

/// Sets the rows the mock engine yields from `fetch_data`.
pub(crate) fn set_data(scanned_columns: Vec<ScannedColumn>, rows: Vec<ScanRow>) {
    DATA.with(|d| *d.borrow_mut() = (scanned_columns, rows));
}

pub(super) struct MockEngine;
pub(super) const ENGINE: MockEngine = MockEngine;

impl ScanEngine for MockEngine {
    fn name(&self) -> &'static str {
        "mock"
    }

    fn fetch_data(&self, _sub_task: &SubTask, scan: &mut ScanFn<'_>) -> Result<Vec<ScannedColumn>> {
        DATA.with(|d| {
            let data = d.borrow();
            for row in &data.1 {
                scan(&data.0, row)?;
            }
            Ok(data.0.clone())
        })
    }
}
