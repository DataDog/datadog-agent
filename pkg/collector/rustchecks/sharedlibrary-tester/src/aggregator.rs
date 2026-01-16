use core::{Aggregator, MetricType, Event};

use crate::cstring::{c_str_to_string, c_str_array_to_vec};

use std::{ffi::{c_char, c_double, c_float, c_int, c_longlong}, sync::atomic::{AtomicUsize, Ordering}};

static METRICS_COUNT: AtomicUsize = AtomicUsize::new(0);
static SERVICE_CHECKS_COUNT: AtomicUsize = AtomicUsize::new(0);
static EVENTS_COUNT: AtomicUsize = AtomicUsize::new(0);
static HISTOGRAM_BUCKET_COUNT: AtomicUsize = AtomicUsize::new(0);
static EVENT_PLATFORM_EVENTS_COUNT: AtomicUsize = AtomicUsize::new(0);

/// Implementation of SubmitMetric that prints the payload
extern "C" fn fake_submit_metric(
    check_id: *mut c_char,
    metric_type: MetricType,
    name: *mut c_char,
    value: c_double,
    tags: *mut *mut c_char,
    hostname: *mut c_char,
    flush_first_value: bool,
) {
    METRICS_COUNT.fetch_add(1, Ordering::SeqCst);

    println!(
        r#"=== Metric ===
{{
    "check_id": "{}",
    "name": "{}",
    "value": {},
    "tags": {:?},
    "host": "{}",
    "type": "{}",
    "flush_first_value": {}
}}"#,
        c_str_to_string(check_id),
        c_str_to_string(name),
        value,
        c_str_array_to_vec(tags),
        c_str_to_string(hostname),
        format!("{:?}", metric_type).to_lowercase(),
        flush_first_value
    );
}

/// Implementation of SubmitServiceCheck that prints the payload
extern "C" fn fake_submit_service_check(
    check_id: *mut c_char,
    name: *mut c_char,
    status: c_int,
    tags: *mut *mut c_char,
    hostname: *mut c_char,
    message: *mut c_char,
) {
    SERVICE_CHECKS_COUNT.fetch_add(1, Ordering::SeqCst);

    println!(
        r#"=== Service Check ===
{{
    "check_id": "{}",
    "name": "{}",
    "status": {},
    "tags": {:?},
    "host": "{}",
    "message": "{}"
}}"#,
        c_str_to_string(check_id),
        c_str_to_string(name),
        status,
        c_str_array_to_vec(tags),
        c_str_to_string(hostname),
        c_str_to_string(message)
    );
}

/// Implementation of SubmitEvent that prints the payload
extern "C" fn fake_submit_event(
    check_id: *mut c_char,
    event_ptr: *const Event,
) {
    EVENTS_COUNT.fetch_add(1, Ordering::SeqCst);

    let event = unsafe { &*event_ptr };

    println!(
        r#"=== Event ===
{{
    "check_id": "{}",
    "event": {:?}
}}"#,
        c_str_to_string(check_id),
        event
    );
}

/// Implementation of SubmitHistogramBucket that prints the payload
extern "C" fn fake_submit_histogram_bucket(
    check_id: *mut c_char,
    metric_name: *mut c_char,
    value: c_longlong,
    lower_bound: c_float,
    upper_bound: c_float,
    monotonic: c_int,
    hostname: *mut c_char,
    tags: *mut *mut c_char,
    flush_first_value: bool,
) {
    HISTOGRAM_BUCKET_COUNT.fetch_add(1, Ordering::SeqCst);

    println!(
        r#"=== Histogram Bucket ===
{{
    "check_id": "{}",
    "metric_name": "{}",
    "value": {},
    "lower_bound": {},
    "upper_bound": {},
    "monotonic": {},
    "hostname": "{}",
    "tags": {:?},
    "flush_first_value": {}
}}"#,
        c_str_to_string(check_id),
        c_str_to_string(metric_name),
        value,
        lower_bound,
        upper_bound,
        monotonic,
        c_str_to_string(hostname),
        c_str_array_to_vec(tags),
        flush_first_value
    );
}

/// Implementation of SubmitEventPlatformEvent that prints the payload
extern "C" fn fake_submit_event_platform_event(
    check_id: *mut c_char,
    raw_event_pointer: *mut c_char,
    raw_event_size: c_int,
    event_type: *mut c_char,
) {
    EVENT_PLATFORM_EVENTS_COUNT.fetch_add(1, Ordering::SeqCst);

    println!(
        r#"=== Event Platform Event ===
{{
    "check_id": "{}",
    "raw_event_pointer": "{}",
    "raw_event_size": {},
    "event_type": "{}"
}}"#,
        c_str_to_string(check_id),
        c_str_to_string(raw_event_pointer),
        raw_event_size,
        c_str_to_string(event_type)
    );
}

pub fn fake_aggregator() -> Aggregator {
    Aggregator::new(
        fake_submit_metric, 
        fake_submit_service_check, 
        fake_submit_event, 
        fake_submit_histogram_bucket, 
        fake_submit_event_platform_event,
    )
}

pub fn print_payload_counts() {
    println!("=== Recap ===");
    println!("Metrics: {}", METRICS_COUNT.load(Ordering::SeqCst));
    println!("Service checks: {}", SERVICE_CHECKS_COUNT.load(Ordering::SeqCst));
    println!("Events: {}", EVENTS_COUNT.load(Ordering::SeqCst));
    println!("Histogram buckets: {}", HISTOGRAM_BUCKET_COUNT.load(Ordering::SeqCst));
    println!("Event platform events: {}", EVENT_PLATFORM_EVENTS_COUNT.load(Ordering::SeqCst));
}
