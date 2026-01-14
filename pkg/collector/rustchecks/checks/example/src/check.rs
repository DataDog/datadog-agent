use anyhow::Result;

use core::*;

/// Check implementation
pub fn check(check: &AgentCheck) -> Result<()> {
    check.gauge("hello.gauge", 1.0, &vec![], "", false)?;
    check.service_check("hello.service_check", ServiceCheckStatus::OK, &vec![], "", "")?;
    check.event("hello.event", "hello.text", 0, "normal", "", &vec![], "info", "", "", "")?;

    Ok(())
}
