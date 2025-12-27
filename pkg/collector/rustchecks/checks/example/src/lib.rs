use rust_check_core::{generate_ffi, AgentCheck};
use std::{error::Error, ffi::CStr};

/// Shared library check version
const VERSION: &'static CStr = c"0.1.0";

/// Check implementation
pub fn check(check: &AgentCheck) -> Result<(), Box<dyn Error>> {
    check.gauge("hello.gauge", 1.0, &vec![], "", false)?;
    check.service_check("hello.service_check", rust_check_core::ServiceCheckStatus::OK, &vec![], "", "")?;
    check.event("hello.event", "hello.text", 0, "normal", "", &vec![], "info", "", "", "")?;

    Ok(())
}

generate_ffi!(check, VERSION);

#[cfg(test)]
mod test {
    use super::*;
    use rust_check_core::test_utils::*;

    #[test]
    fn test_check() -> Result<(), Box<dyn Error>> {    
        let agent_check = AgentCheck::new("mock_check_id", "","", mock_aggregator())?;
        check(&agent_check)
    }
}
