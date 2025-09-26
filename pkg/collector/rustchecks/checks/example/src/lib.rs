mod ffi;
use rust_check_core::AgentCheck;

use std::error::Error;

pub trait CheckImplementation {
    fn check(&self) -> Result<(), Box<dyn Error>>;
}

impl CheckImplementation for AgentCheck {
    /// Check implementation
    fn check(&self) -> Result<(), Box<dyn Error>> {
        self.gauge("hello.world", 1.0, &vec![], "", false);

        Ok(())
    }
}
