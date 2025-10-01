mod ffi;
use rust_check_core::{AgentCheck, ServiceCheckStatus};

use std::{error::Error, vec};

pub trait CheckImplementation {
    fn check(&self) -> Result<(), Box<dyn Error>>;
}

impl CheckImplementation for AgentCheck {
    /// Check implementation
    fn check(&self) -> Result<(), Box<dyn Error>> {
        self.gauge("hello.metric", 1.0, &vec![], "", false)?;
        self.service_check("hello.service_check", ServiceCheckStatus::OK, &vec![], "", "hello")?;
        self.event("hello", "hello world", 123, "priority", "", &vec![], "alert_type", "aggregation_key", "source_type_name", "event_type")?;

        Ok(())
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use rust_check_core::test_utils::*;

    #[test]
    fn test_check_implementation() -> Result<(), Box<dyn Error>> {
        let agent_check = mock_agent_check();
        agent_check.check()
    }
}
