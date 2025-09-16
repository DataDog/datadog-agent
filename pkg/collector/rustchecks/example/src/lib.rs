#![allow(unused_imports)]

mod core;
use core::check::{AgentCheck, ServiceCheckStatus};

use std::error::Error;

impl AgentCheck {
    /// Check implementation
    pub fn check(self) -> Result<(), Box<dyn Error>> {
        self.gauge("hello.world", 1.0, &vec![], "", false);

        Ok(())
    }
}
