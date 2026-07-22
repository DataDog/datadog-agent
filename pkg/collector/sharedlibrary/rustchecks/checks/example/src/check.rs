use anyhow::{Ok, Result};

use core::*;

/// Check implementation
pub fn check(check: &AgentCheck) -> Result<()> {
    check.gauge("hello.gauge", 1.0, &Vec::new(), "", false)?;
    check.service_check(
        "hello.service_check",
        ServiceCheckStatus::OK,
        &Vec::new(),
        "",
        "",
    )?;
    check.event(
        "hello.event",
        "hello.text",
        0,
        "normal",
        "",
        &Vec::new(),
        "info",
        "",
        "",
        "",
    )?;

    Ok(())
}

#[cfg(test)]
mod tests {
    // TODO: replace with a real assertion once a test stub for AgentCheck exists.
    #[test]
    fn test_check_placeholder() {
        assert_eq!(1, 1);
    }
}
