use super::aggregator::{CheckInstance, MetricType, Aggregator};

pub type Instance = CheckInstance;

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
    pub fn new(instance_ptr: *const Instance) -> Self {
        let instance = unsafe { &*instance_ptr };

        let check_id  = instance.get_check_id();

        let aggregator  = instance.get_callbacks();

        AgentCheck { check_id, aggregator }
    }

    pub fn print_check_id(&self) {
        println!("Check ID: {}", self.check_id);
    }

    pub fn print_aggregator(&self) {
        println!("Aggregator: {:?}", self.aggregator);
    }

    // TODO: maybe use Option for optional arguments (tags, hostname, flush_first_value)

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
