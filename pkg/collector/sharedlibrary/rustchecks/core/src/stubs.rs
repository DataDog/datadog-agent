//! Test harness mirroring integrations-core's `AggregatorStub`: records what a
//! check submits and exposes assertions over it. Submissions are captured in
//! thread-local storage, so parallel `#[test]`s stay isolated.

use std::cell::RefCell;
use std::ffi::{c_char, c_double, c_float, c_int, c_longlong};

use crate::agent_check::AgentCheck;
use crate::aggregator::{Aggregator, Event, LogLevel, MetricType, ServiceCheckStatus};
use crate::config::Config;
use crate::cstring::to_rust_string;

#[derive(Clone, Debug, PartialEq)]
pub struct RecordedMetric {
    pub metric_type: MetricType,
    pub name: String,
    pub value: f64,
    pub tags: Vec<String>,
    pub hostname: String,
    pub flush_first_value: bool,
}

#[derive(Clone, Debug, PartialEq)]
pub struct RecordedServiceCheck {
    pub name: String,
    pub status: ServiceCheckStatus,
    pub tags: Vec<String>,
    pub hostname: String,
    pub message: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct RecordedEvent {
    pub title: String,
    pub text: String,
    pub timestamp: i64,
    pub priority: String,
    pub host: String,
    pub tags: Vec<String>,
    pub alert_type: String,
    pub aggregation_key: String,
    pub source_type_name: String,
    pub event_type: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct RecordedHistogramBucket {
    pub name: String,
    pub value: i64,
    pub lower_bound: f32,
    pub upper_bound: f32,
    pub monotonic: bool,
    pub hostname: String,
    pub tags: Vec<String>,
    pub flush_first_value: bool,
}

#[derive(Clone, Debug, PartialEq)]
pub struct RecordedEventPlatformEvent {
    pub raw_event: Vec<u8>,
    pub event_type: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct RecordedLog {
    pub message: String,
    /// Raw rtloader `log_level_t` value (see [`crate::LogLevel`]).
    pub level: c_int,
}

#[derive(Default)]
struct Recorded {
    metrics: Vec<RecordedMetric>,
    service_checks: Vec<RecordedServiceCheck>,
    events: Vec<RecordedEvent>,
    histogram_buckets: Vec<RecordedHistogramBucket>,
    event_platform_events: Vec<RecordedEventPlatformEvent>,
    logs: Vec<RecordedLog>,
}

thread_local! {
    static RECORDED: RefCell<Recorded> = RefCell::new(Recorded::default());
}

/// Copies eagerly: the callbacks free their C strings right after invoking us.
/// A null/invalid pointer becomes an empty string.
fn read_cstr(ptr: *const c_char) -> String {
    to_rust_string(ptr).unwrap_or_default()
}

fn read_cstr_array(ptr: *mut *mut c_char) -> Vec<String> {
    if ptr.is_null() {
        return Vec::new();
    }

    let mut out = Vec::new();
    let mut current = ptr;
    unsafe {
        while !(*current).is_null() {
            out.push(read_cstr(*current));
            current = current.add(1);
        }
    }
    out
}

fn service_check_status_from_int(value: c_int) -> ServiceCheckStatus {
    match value {
        0 => ServiceCheckStatus::OK,
        1 => ServiceCheckStatus::WARNING,
        2 => ServiceCheckStatus::CRITICAL,
        _ => ServiceCheckStatus::UNKNOWN,
    }
}

// Recording callbacks, matching the `Aggregator` C-ABI signatures.

#[allow(clippy::too_many_arguments)]
extern "C" fn record_metric(
    _check_id: *mut c_char,
    metric_type: MetricType,
    name: *mut c_char,
    value: c_double,
    tags: *mut *mut c_char,
    hostname: *mut c_char,
    flush_first_value: bool,
) {
    let metric = RecordedMetric {
        metric_type,
        name: read_cstr(name),
        value,
        tags: read_cstr_array(tags),
        hostname: read_cstr(hostname),
        flush_first_value,
    };
    RECORDED.with(|r| r.borrow_mut().metrics.push(metric));
}

extern "C" fn record_service_check(
    _check_id: *mut c_char,
    name: *mut c_char,
    status: c_int,
    tags: *mut *mut c_char,
    hostname: *mut c_char,
    message: *mut c_char,
) {
    let service_check = RecordedServiceCheck {
        name: read_cstr(name),
        status: service_check_status_from_int(status),
        tags: read_cstr_array(tags),
        hostname: read_cstr(hostname),
        message: read_cstr(message),
    };
    RECORDED.with(|r| r.borrow_mut().service_checks.push(service_check));
}

extern "C" fn record_event(_check_id: *mut c_char, event: *const Event) {
    if event.is_null() {
        return;
    }

    // Safety: the aggregator passes a valid pointer live for the call.
    let event = unsafe { &*event };
    let recorded = RecordedEvent {
        title: read_cstr(event.title),
        text: read_cstr(event.text),
        timestamp: event.timestamp,
        priority: read_cstr(event.priority),
        host: read_cstr(event.host),
        tags: read_cstr_array(event.tags),
        alert_type: read_cstr(event.alert_type),
        aggregation_key: read_cstr(event.aggregation_key),
        source_type_name: read_cstr(event.source_type_name),
        event_type: read_cstr(event.event_type),
    };
    RECORDED.with(|r| r.borrow_mut().events.push(recorded));
}

#[allow(clippy::too_many_arguments)]
extern "C" fn record_histogram_bucket(
    _check_id: *mut c_char,
    name: *mut c_char,
    value: c_longlong,
    lower_bound: c_float,
    upper_bound: c_float,
    monotonic: c_int,
    hostname: *mut c_char,
    tags: *mut *mut c_char,
    flush_first_value: bool,
) {
    let bucket = RecordedHistogramBucket {
        name: read_cstr(name),
        value,
        lower_bound,
        upper_bound,
        monotonic: monotonic != 0,
        hostname: read_cstr(hostname),
        tags: read_cstr_array(tags),
        flush_first_value,
    };
    RECORDED.with(|r| r.borrow_mut().histogram_buckets.push(bucket));
}

extern "C" fn record_event_platform_event(
    _check_id: *mut c_char,
    raw_event: *mut c_char,
    raw_event_size: c_int,
    event_type: *mut c_char,
) {
    // Read by length: the payload may be arbitrary bytes (e.g. protobuf with NULs).
    let raw_event = if raw_event.is_null() || raw_event_size < 0 {
        Vec::new()
    } else {
        unsafe {
            std::slice::from_raw_parts(raw_event as *const u8, raw_event_size as usize).to_vec()
        }
    };
    let recorded = RecordedEventPlatformEvent {
        raw_event,
        event_type: read_cstr(event_type),
    };
    RECORDED.with(|r| r.borrow_mut().event_platform_events.push(recorded));
}

extern "C" fn log_msg(message: *mut c_char, level: c_int) {
    let recorded = RecordedLog {
        message: read_cstr(message),
        level,
    };
    RECORDED.with(|r| r.borrow_mut().logs.push(recorded));
}

/// Records everything a check submits and exposes assertions over it.
pub struct AggregatorStub {
    check_id: String,
}

impl Default for AggregatorStub {
    fn default() -> Self {
        Self::new()
    }
}

impl AggregatorStub {
    /// Creates a stub and clears any submissions recorded on this thread.
    pub fn new() -> Self {
        RECORDED.with(|r| *r.borrow_mut() = Recorded::default());
        Self {
            check_id: "test-check".to_string(),
        }
    }

    /// Builds the recording [`Aggregator`] wired to this stub's callbacks.
    pub fn aggregator(&self) -> Aggregator {
        Aggregator::new(
            record_metric,
            record_service_check,
            record_event,
            record_histogram_bucket,
            record_event_platform_event,
            log_msg,
        )
    }

    /// Builds an [`AgentCheck`] wired to this stub. Panics on invalid YAML.
    pub fn agent_check(&self, init_config: &str, instance_config: &str) -> AgentCheck {
        AgentCheck::new(
            self.check_id.clone(),
            Config::from_str(init_config).expect("invalid init_config YAML"),
            Config::from_str(instance_config).expect("invalid instance_config YAML"),
            self.aggregator(),
        )
    }

    /// [`agent_check`](Self::agent_check) with empty configs.
    pub fn agent_check_default(&self) -> AgentCheck {
        self.agent_check("{}", "{}")
    }

    pub fn metrics(&self) -> Vec<RecordedMetric> {
        RECORDED.with(|r| r.borrow().metrics.clone())
    }

    pub fn service_checks(&self) -> Vec<RecordedServiceCheck> {
        RECORDED.with(|r| r.borrow().service_checks.clone())
    }

    pub fn events(&self) -> Vec<RecordedEvent> {
        RECORDED.with(|r| r.borrow().events.clone())
    }

    pub fn histogram_buckets(&self) -> Vec<RecordedHistogramBucket> {
        RECORDED.with(|r| r.borrow().histogram_buckets.clone())
    }

    pub fn event_platform_events(&self) -> Vec<RecordedEventPlatformEvent> {
        RECORDED.with(|r| r.borrow().event_platform_events.clone())
    }

    pub fn logs(&self) -> Vec<RecordedLog> {
        RECORDED.with(|r| r.borrow().logs.clone())
    }

    /// Asserts a metric with `name` and `value` was submitted (any type/tags).
    pub fn assert_metric(&self, name: &str, value: f64) {
        let found = self
            .metrics()
            .iter()
            .any(|m| m.name == name && m.value == value);
        assert!(
            found,
            "expected a metric name={name:?} value={value}, but got: {:#?}",
            self.metrics()
        );
    }

    /// Asserts a metric with `name`, `metric_type` and `value` was submitted.
    pub fn assert_metric_with_type(&self, name: &str, metric_type: MetricType, value: f64) {
        let found = self
            .metrics()
            .iter()
            .any(|m| m.name == name && m.metric_type == metric_type && m.value == value);
        assert!(
            found,
            "expected a metric name={name:?} type={metric_type:?} value={value}, but got: {:#?}",
            self.metrics()
        );
    }

    /// Asserts a metric with `name`, `value` and exactly `tags` (unordered).
    pub fn assert_metric_with_tags(&self, name: &str, value: f64, tags: &[&str]) {
        let found = self
            .metrics()
            .iter()
            .any(|m| m.name == name && m.value == value && tags_match(&m.tags, tags));
        assert!(
            found,
            "expected a metric name={name:?} value={value} tags={tags:?}, but got: {:#?}",
            self.metrics()
        );
    }

    /// Asserts the number of recorded metrics named `name`.
    pub fn assert_metric_count(&self, name: &str, count: usize) {
        let actual = self.metrics().iter().filter(|m| m.name == name).count();
        assert_eq!(
            actual,
            count,
            "expected {count} metric(s) named {name:?}, found {actual}: {:#?}",
            self.metrics()
        );
    }

    /// Asserts a service check with `name` and `status` was submitted.
    pub fn assert_service_check(&self, name: &str, status: ServiceCheckStatus) {
        let found = self
            .service_checks()
            .iter()
            .any(|sc| sc.name == name && sc.status == status);
        assert!(
            found,
            "expected a service check name={name:?} status={status:?}, but got: {:#?}",
            self.service_checks()
        );
    }

    /// Asserts an event with `title` and `text` was submitted.
    pub fn assert_event(&self, title: &str, text: &str) {
        let found = self
            .events()
            .iter()
            .any(|e| e.title == title && e.text == text);
        assert!(
            found,
            "expected an event title={title:?} text={text:?}, but got: {:#?}",
            self.events()
        );
    }

    /// Asserts a log line with `level` and `message` was emitted.
    pub fn assert_log(&self, level: LogLevel, message: &str) {
        let level = level as c_int;
        let found = self
            .logs()
            .iter()
            .any(|l| l.level == level && l.message == message);
        assert!(
            found,
            "expected a log level={level} message={message:?}, but got: {:#?}",
            self.logs()
        );
    }
}

fn tags_match(actual: &[String], expected: &[&str]) -> bool {
    if actual.len() != expected.len() {
        return false;
    }
    expected.iter().all(|t| actual.iter().any(|a| a == t))
}
