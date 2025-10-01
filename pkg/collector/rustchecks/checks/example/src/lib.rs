use rust_check_core::{generate_ffi, AgentCheck, ServiceCheckStatus};
use std::error::Error;

/// Check implementation
pub fn check_implementation(check: &AgentCheck) -> Result<(), Box<dyn Error>> {
    check.gauge("hello.metric", 1.0, &vec![], "", false)?;
    check.service_check("hello.service_check", ServiceCheckStatus::OK, &vec![], "", "hello")?;
    check.event("hello", "hello world", 123, "priority", "", &vec![], "alert_type", "aggregation_key", "source_type_name", "event_type")?;

    Ok(())
}

generate_ffi!(check_implementation);

#[cfg(test)]
mod test {
    use super::*;
    use rust_check_core::test_utils::*;

    #[test]
    fn test_check_implementation() -> Result<(), Box<dyn Error>> {
        let agent_check = mock_agent_check();
        check_implementation(&agent_check)
    }
}
