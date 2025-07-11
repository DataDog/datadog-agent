use super::aggregator::{MetricType, Aggregator};
use std::ffi::{c_char, CString};

pub type CheckID = *mut c_char;

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

    // TODO: use Into<String> trait to allow passing any type of string that can be converted to String ???
    // use Option for optional arguments (tags, hostname, flush_first_value)

    // metric functions
    pub fn gauge(&mut self, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Gauge, name, value, tags, hostname, flush_first_value);
    }

    pub fn rate(&mut self, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Rate, name, value, tags, hostname, flush_first_value);
    }

    pub fn count(&mut self, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Count, name, value, tags, hostname, flush_first_value);
    }

    pub fn monotonic_count(&mut self, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::MonotonicCount, name, value, tags, hostname, flush_first_value);
    }

    pub fn decrement(&mut self, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Counter, name, value, tags, hostname, flush_first_value);
    }

    pub fn histogram(&mut self, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Histogram, name, value, tags, hostname, flush_first_value);
    }

    pub fn historate(&mut self, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
        self.aggregator.submit_metric(&self.check_id, MetricType::Historate, name, value, tags, hostname, flush_first_value);
    }

    // service check functions
    pub fn service_check(&mut self, name: &String, status: i32, tags: &Vec<String>, hostname: &String, message: &String) {
        self.aggregator.submit_service_check(&self.check_id, name, status, tags, hostname, message);
    }
}
