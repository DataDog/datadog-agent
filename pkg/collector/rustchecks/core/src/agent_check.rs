use anyhow::Result;

use super::aggregator::{Aggregator, MetricType, ServiceCheckStatus};
use super::config::Config;

use std::ffi::{c_int, c_long};

pub struct AgentCheck {
    check_id: String,           // corresponding id in the Agent
    aggregator: Aggregator,     // submit callbacks
    
    // these fields are made public to mimic the way configurations are used in Python checks
    pub init_config: Config,    // common check configuration
    pub instance: Config,       // instance specific configuration
}

impl AgentCheck {
    pub fn new(check_id: &str, init_config_str: &str, instance_config_str: &str, aggregator: Aggregator) -> Result<Self> {
        let init_config = Config::new(&init_config_str)?;
        let instance = Config::new(&instance_config_str)?;

        let agent_check = Self { check_id: check_id.to_string(), aggregator, init_config, instance };
        Ok(agent_check)
    }

    /// Send Gauge metric
    pub fn gauge(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_metric(&self.check_id, MetricType::Gauge, name, value, tags, hostname, flush_first_value)
    }

    /// Send Rate metric
    pub fn rate(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_metric(&self.check_id, MetricType::Rate, name, value, tags, hostname, flush_first_value)
    }

    /// Send Count metric
    pub fn count(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_metric(&self.check_id, MetricType::Count, name, value, tags, hostname, flush_first_value)
    }

    /// Send Monotonic Count metric
    pub fn monotonic_count(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_metric(&self.check_id, MetricType::MonotonicCount, name, value, tags, hostname, flush_first_value)
    }

    /// Send Decrement metric
    pub fn decrement(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_metric(&self.check_id, MetricType::Counter, name, value, tags, hostname, flush_first_value)
    }

    /// Send Histogram metric
    pub fn histogram(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_metric(&self.check_id, MetricType::Histogram, name, value, tags, hostname, flush_first_value)
    }

    /// Send Historate metric
    pub fn historate(&self, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_metric(&self.check_id, MetricType::Historate, name, value, tags, hostname, flush_first_value)
    }

    /// Send Servive Check
    pub fn service_check(&self, name: &str, status: ServiceCheckStatus, tags: &[String], hostname: &str, message: &str) -> Result<()> {
        self.aggregator.submit_service_check(&self.check_id, name, status, tags, hostname, message)
    }

    /// Send Event
    pub fn event(&self, title: &str, text: &str, timestamp: c_long, priority: &str, host: &str, tags: &[String], alert_type: &str, aggregation_key: &str, source_type_name: &str, event_type: &str) -> Result<()> {
        self.aggregator.submit_event(&self.check_id, title, text, timestamp, priority, host, tags, alert_type, aggregation_key, source_type_name, event_type)
    }

    /// Send Histogram Bucket
    pub fn submit_histogram_bucket(&self, metric_name: &str, value: i64, lower_bound: f32, upper_bound: f32, monotonic: c_int, hostname: &str, tags: &[String], flush_first_value: bool) -> Result<()> {
        self.aggregator.submit_histogram_bucket(&self.check_id, metric_name, value, lower_bound, upper_bound, monotonic, hostname, tags, flush_first_value)
    }

    /// Send Event Platform Event
    pub fn event_platform_event(&self, raw_event: &str, event_track_type: &str) -> Result<()> {
        self.aggregator.submit_event_platform_event(&self.check_id, raw_event, raw_event.len() as c_int, event_track_type)
    }
}
