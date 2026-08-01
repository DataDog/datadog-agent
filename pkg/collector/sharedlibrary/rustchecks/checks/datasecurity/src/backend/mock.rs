//! Test-only scan engine (`platform: mock`) returning caller-controlled data.

use std::cell::RefCell;

use anyhow::Result;
use serde_json::Value;

use super::{ScanData, ScanEngine};
use crate::config::SubTask;

thread_local! {
    static DATA: RefCell<Value> = const { RefCell::new(Value::Null) };
}

/// Sets the `{ column: [values] }` map the mock engine returns from `fetch_data`.
pub(crate) fn set_data(data: Value) {
    DATA.with(|d| *d.borrow_mut() = data);
}

pub(super) struct MockEngine;
pub(super) const ENGINE: MockEngine = MockEngine;

impl ScanEngine for MockEngine {
    fn name(&self) -> &'static str {
        "mock"
    }

    fn fetch_data(&self, _sub_task: &SubTask) -> Result<ScanData> {
        Ok(ScanData {
            columns: DATA.with(|d| d.borrow().clone()),
            ..Default::default()
        })
    }
}
