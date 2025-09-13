#![allow(dead_code)]

use super::aggregator::{Instance, MetricType, Aggregator};

use std::error::Error;

#[repr(i32)]
pub enum ServiceCheckStatus {
    OK = 0,
    WARNING = 1,
    CRITICAL = 2,
    UNKNOWN = 3,
}

pub struct AgentCheck {
    check_id: String,
    aggregator: Aggregator, // submit callbacks

    // the field is public to match Python checks' syntax for getting Instance parameters
    pub instance: Instance, // used to get specific check parameters
}

impl AgentCheck {
    pub fn new(instance: Instance, aggregator: Aggregator) -> Result<Self, Box<dyn Error>> {
        // required parameters for every check
        let check_id = instance.get("check_id")?;

        Ok(Self { check_id, aggregator, instance })
    }

    /// Send Gauge metric
    pub fn gauge(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Gauge, name, value, tags, hostname, flush_first_value);
    }

    /// Send Rate metric
    pub fn rate(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Rate, name, value, tags, hostname, flush_first_value);
    }

    /// Send Count metric
    pub fn count(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Count, name, value, tags, hostname, flush_first_value);
    }

    /// Send Monotonic Count metric
    pub fn monotonic_count(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::MonotonicCount, name, value, tags, hostname, flush_first_value);
    }

    /// Send Decrement metric
    pub fn decrement(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Counter, name, value, tags, hostname, flush_first_value);
    }

    /// Send Histogram metric
    pub fn histogram(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Histogram, name, value, tags, hostname, flush_first_value);
    }

    /// Send Historate metric
    pub fn historate(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Historate, name, value, tags, hostname, flush_first_value);
    }

    /// Send Servive Check
    pub fn service_check(&self, name: &str, status: ServiceCheckStatus, tags: &[String], hostname: &str, message: &str) {
        self.aggregator.submit_service_check(&self.check_id, name, status as i32, tags, hostname, message);
    }
}
