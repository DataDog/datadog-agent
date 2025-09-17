#![allow(dead_code)]

use super::aggregator::{Config, Aggregator, MetricType, Event};

use std::error::Error;
use std::ffi::{c_char, CStr};

#[repr(i32)]
pub enum ServiceCheckStatus {
    OK = 0,
    WARNING = 1,
    CRITICAL = 2,
    UNKNOWN = 3,
}

pub struct AgentCheck {
    check_id: String,           // corresponding id in the Agent
    aggregator: Aggregator,     // submit callbacks
    // these fields are made public to mimic the way configurations are used in Python checks
    pub init_config: Config,    // common check configuration
    pub instance: Config,       // instance specific configuration
}

impl AgentCheck {
    pub fn new(check_id_str: *const c_char, init_config_str: *const c_char, instance_config_str: *const c_char, aggregator_ptr: *const Aggregator) -> Result<Self, Box<dyn Error>> {
        let check_id = unsafe { CStr::from_ptr(check_id_str) }
            .to_str()?
            .to_string();
        
        // parse configuration strings
        let init_config = Config::from_str(init_config_str)?;
        let instance = Config::from_str(instance_config_str)?;
        
        // gather callbacks in a struct
        let aggregator = Aggregator::from_raw(aggregator_ptr);

        Ok(Self { check_id, aggregator, init_config, instance })
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

    /// Send Event
    pub fn event(&self, event: &Event) {
        self.aggregator.submit_event(&self.check_id, event);
    }

    /// Send Histogram Bucket
    pub fn submit_histogram_bucket(&self, metric_name: &str, value: i64, lower_bound: f32, upper_bound: f32, monotonic: i32, hostname: &str, tags: &[String], flush_first_value: bool) {
        self.aggregator.submit_histogram_bucket(&self.check_id, metric_name, value, lower_bound, upper_bound, monotonic, hostname, tags, flush_first_value);
    }

    /// Send Event Platform Evemt
    pub fn submit_event_platform_event(&self, raw_event_pointer: &str, raw_event_size: i32, event_type: &str) {
        self.aggregator.submit_event_platform_event(&self.check_id, raw_event_pointer, raw_event_size, event_type);
    }
}
