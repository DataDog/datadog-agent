use super::aggregator::{MetricType, Aggregator};

use std::ffi::{c_char, CString};

pub type CheckID = *mut c_char;

#[repr(i32)]
pub enum ServiceCheckStatus {
    OK = 0,
    WARNING = 1,
    CRITICAL = 2,
    UNKNOWN = 3,
}

pub struct AgentCheck {
    check_id: String,
    aggregator: Aggregator,
}

impl AgentCheck {
    pub fn new(check_id: *mut c_char) -> Self {
        let check_id  = unsafe { CString::from_raw(check_id) }.into_string().expect("Failed to convert check_id to String");
        let aggregator = Aggregator::new();
        
        AgentCheck { check_id, aggregator }
    }

    // TODO: maybe use Into<String> trait to allow passing any type of string that can be converted to String ???
    // use Option for optional arguments (tags, hostname, flush_first_value)

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
