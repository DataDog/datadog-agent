use crate::aggregator::{Aggregator, MetricType, Event};

use std::ffi::{c_char, c_double, c_float, c_longlong, c_int, CStr};

/// Helper function to safely convert C string to Rust string for printing
fn c_str_to_string(ptr: *mut c_char) -> String {
    if ptr.is_null() {
        "NULL".to_string()
    } else {
        unsafe { CStr::from_ptr(ptr) }
            .to_str()
            .unwrap_or("<invalid_utf8>")
            .to_string()
    }
}

/// Helper function to print C string array
fn c_str_array_to_vec(ptr: *mut *mut c_char) -> Vec<String> {
    if ptr.is_null() {
        return vec!["NULL".to_string()];
    }
    
    let mut result = Vec::new();
    let mut current = ptr;
    
    
    unsafe {
        while !(*current).is_null() {
            result.push(c_str_to_string(*current));
            current = current.add(1);
        }
    }
    
    if result.is_empty() {
        vec!["<empty_array>".to_string()]
    } else {
        result
    }
}

/// Mock implementation of SubmitMetric
extern "C" fn mock_submit_metric(
    check_id: *mut c_char,
    metric_type: MetricType,
    name: *mut c_char,
    value: c_double,
    tags: *mut *mut c_char,
    hostname: *mut c_char,
    flush_first_value: bool,
) {
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

/// Mock implementation of SubmitServiceCheck
extern "C" fn mock_submit_service_check(
    check_id: *mut c_char,
    name: *mut c_char,
    status: c_int,
    tags: *mut *mut c_char,
    hostname: *mut c_char,
    message: *mut c_char,
) {
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

/// Mock implementation of SubmitEvent
extern "C" fn mock_submit_event(
    check_id: *mut c_char,
    event_ptr: *const Event,
) {
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

/// Mock implementation of SubmitHistogramBucket
extern "C" fn mock_submit_histogram_bucket(
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

/// Mock implementation of SubmitEventPlatformEvent
extern "C" fn mock_submit_event_platform_event(
    check_id: *mut c_char,
    raw_event_pointer: *mut c_char,
    raw_event_size: c_int,
    event_type: *mut c_char,
) {
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

pub fn mock_aggregator() -> Aggregator {
    Aggregator::new(
        mock_submit_metric, 
        mock_submit_service_check, 
        mock_submit_event, 
        mock_submit_histogram_bucket, 
        mock_submit_event_platform_event
    )
}
