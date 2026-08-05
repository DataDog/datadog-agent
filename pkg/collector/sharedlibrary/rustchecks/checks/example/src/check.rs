use anyhow::{Ok, Result};

use shlib_core::*;

/// Check implementation
pub fn check(check: &AgentCheck) -> Result<()> {
    check.log(LogLevel::Info, "hello: example check running");

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
    use super::check;

    use core::stubs::{AggregatorStub, RecordedEvent};
    use core::{LogLevel, MetricType, ServiceCheckStatus};

    fn run_check() -> AggregatorStub {
        let aggregator = AggregatorStub::new();
        let agent_check = aggregator.agent_check_default();

        check(&agent_check).expect("check run should not fail");

        aggregator
    }

    #[test]
    fn emits_hello_gauge() {
        let aggregator = run_check();

        aggregator.assert_metric("hello.gauge", 1.0);
        aggregator.assert_metric_with_type("hello.gauge", MetricType::Gauge, 1.0);
        aggregator.assert_metric_count("hello.gauge", 1);
    }

    #[test]
    fn emits_hello_log() {
        let aggregator = run_check();

        aggregator.assert_log(LogLevel::Info, "hello: example check running");
    }

    #[test]
    fn emits_hello_service_check() {
        let aggregator = run_check();

        aggregator.assert_service_check("hello.service_check", ServiceCheckStatus::OK);
    }

    #[test]
    fn emits_hello_event() {
        let aggregator = run_check();

        assert_eq!(
            aggregator.events(),
            vec![RecordedEvent {
                title: "hello.event".to_string(),
                text: "hello.text".to_string(),
                timestamp: 0,
                priority: "normal".to_string(),
                host: String::new(),
                tags: Vec::new(),
                alert_type: "info".to_string(),
                aggregation_key: String::new(),
                source_type_name: String::new(),
                event_type: String::new(),
            }]
        );
    }

    #[test]
    fn emits_exactly_the_expected_submissions() {
        let aggregator = run_check();

        assert_eq!(aggregator.metrics().len(), 1);
        assert_eq!(aggregator.service_checks().len(), 1);
        assert_eq!(aggregator.events().len(), 1);
        assert_eq!(aggregator.logs().len(), 1);
        assert!(aggregator.histogram_buckets().is_empty());
        assert!(aggregator.event_platform_events().is_empty());
    }
}
