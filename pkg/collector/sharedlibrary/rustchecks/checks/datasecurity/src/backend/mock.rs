//! Test-only scan engine (`platform: mock`) returning caller-controlled data.

use std::cell::RefCell;

use anyhow::Result;

use super::{ScanData, ScanEngine};
use crate::config::SubTask;

thread_local! {
    static DATA: RefCell<ScanData> = RefCell::new(ScanData::default());
}

/// Sets the [`ScanData`] the mock engine returns from `fetch_data`.
pub(crate) fn set_data(data: ScanData) {
    DATA.with(|d| *d.borrow_mut() = data);
}

pub(super) struct MockEngine;
pub(super) const ENGINE: MockEngine = MockEngine;

impl ScanEngine for MockEngine {
    fn name(&self) -> &'static str {
        "mock"
    }

    fn fetch_data(&self, _sub_task: &SubTask) -> Result<ScanData> {
        Ok(DATA.with(|d| d.borrow().clone()))
    }
}
