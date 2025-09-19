#![allow(unused_imports)]

mod ffi;
use datadog_agent_core::{AgentCheck, ServiceCheckStatus};

use std::error::Error;

pub trait CheckImplementation {
    fn check(self) -> Result<(), Box<dyn Error>>;
}

impl CheckImplementation for AgentCheck {
    /// Check implementation
    fn check(self) -> Result<(), Box<dyn Error>> {
        self.gauge("hello.world", 1.0, &vec![], "", false);

        Ok(())
    }
}
