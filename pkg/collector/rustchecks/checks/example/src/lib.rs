use rust_check_core::{generate_ffi, AgentCheck};
use std::error::Error;

/// Check implementation
pub fn check_implementation(check: &AgentCheck) -> Result<(), Box<dyn Error>> {
    check.gauge("hello.gauge", 1.0, &vec![], "", false)?;

    Ok(())
}

generate_ffi!(check_implementation);

#[cfg(test)]
mod test {
    use super::*;
    use rust_check_core::test_utils::*;

    #[test]
    fn test_check_implementation() -> Result<(), Box<dyn Error>> {    
        let check = AgentCheck::new("mock_check_id", "","", mock_aggregator())?;
        check_implementation(&check)
    }
}
