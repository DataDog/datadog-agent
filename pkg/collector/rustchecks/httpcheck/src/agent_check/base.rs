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
    aggregator: &'static Aggregator, // submit callbacks
    pub instance: Instance, // used to get specific check parameters
}

impl AgentCheck {
    pub fn new(instance_str: &str, aggregator: &'static Aggregator) -> Result<Self, Box<dyn Error>> {
        let instance = Instance::new(instance_str)?;

        // required parameters
        let check_id = instance.get("check_id")?;

        Ok(Self { check_id, aggregator, instance })
    }

    // metric functions
    pub fn gauge(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Gauge, name, value, tags, hostname, flush_first_value);
    }

    pub fn rate(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Rate, name, value, tags, hostname, flush_first_value);
    }

    pub fn count(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Count, name, value, tags, hostname, flush_first_value);
    }

    pub fn monotonic_count(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::MonotonicCount, name, value, tags, hostname, flush_first_value);
    }

    pub fn decrement(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Counter, name, value, tags, hostname, flush_first_value);
    }

    pub fn histogram(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Histogram, name, value, tags, hostname, flush_first_value);
    }

    pub fn historate(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Historate, name, value, tags, hostname, flush_first_value);
    }

    // service check functions
    pub fn service_check(&self, name: &str, status: ServiceCheckStatus, tags: &[String], hostname: &str, message: &str) {
        self.aggregator.submit_service_check(&self.check_id, name, status as i32, tags, hostname, message);
    }
}
