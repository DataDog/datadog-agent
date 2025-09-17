#![allow(dead_code)]

use super::cstring::*;

use std::collections::HashMap;
use std::ffi::{c_char, c_double, c_float, c_int, c_long, c_longlong, CStr};
use std::error::Error;

use serde_yaml::Value;
use serde::de::DeserializeOwned;

/// Replica of the Agent metric type enum
#[repr(C)]
pub enum MetricType {
    Gauge = 0,
    Rate = 1,
    Count = 2,
    MonotonicCount = 3,
    Counter = 4,
    Histogram = 5,
    Historate = 6,
}

/// Replica of the Agent event struct
#[repr(C)]
pub struct Event {
    title: *mut c_char,
    text: *mut c_char,
    timestamp: c_long,
    priority: *mut c_char,
    host: *mut c_char,
    tags: *mut *mut c_char,
    alert_type: *mut c_char,
    aggregation_key: *mut c_char,
    source_type_name: *mut c_char,
    event_type: *mut c_char,
}

/// Signature of the submit metric function
type SubmitMetric = extern "C" fn(
    *mut c_char,        // check id
    MetricType,         // metric type
    *mut c_char,        // name
    c_double,           // value
    *mut *mut c_char,   // tags
    *mut c_char,        // hostname
    bool,               // flush first value
);

/// Signature of the submit service check function
type SubmitServiceCheck = extern "C" fn(
    *mut c_char,        // check id
    *mut c_char,        // name
    c_int,              // status
    *mut *mut c_char,   // tags
    *mut c_char,        // hostname
    *mut c_char,        // message
);

/// Signature of the submit event function
type SubmitEvent = extern "C" fn(
    *mut c_char,        // check_id
    *const Event,       // event
);
/// Signature of the submit histogram bucket function
type SubmitHistogramBucket = extern "C" fn(
    *mut c_char,        // check_id
    *mut c_char,        // metric name
    c_longlong,         // value
    c_float,            // lower bound
    c_float,            // upper bound
    c_int,              // monotonic
    *mut c_char,        // hostname
    *mut *mut c_char,   // tags
    bool,               // flush first value
);

/// Signature of the submit event platform event function
type SubmitEventPlatformEvent = extern "C" fn(
    *mut c_char, // check_id
    *mut c_char, // raw event pointer
    c_int,       // raw event size
    *mut c_char, // event type
);

/// Represents the parameters passed by the Agent to the check
/// 
/// It stores every parameter in a map using `Serde` and provide a method for retrieving the values
#[repr(C)]
pub struct Config {
    map: HashMap<String, Value>,
}

impl Config {
    pub fn from_str(cstr: *const c_char) -> Result<Self, Box<dyn Error>> {
        // read string
        let str = unsafe { CStr::from_ptr(cstr) }.to_str().unwrap_or("");

        // create map of parameters from the string
        let map: HashMap<String, Value> = match serde_yaml::from_str(str) {
            Ok(map) => map,
            Err(_) => HashMap::new(),
        };

        Ok(Self { map })
    }

    pub fn get<T>(&self, key: &str) -> Result<T, Box<dyn Error>>
    where 
        T: DeserializeOwned,
    {
        match self.map.get(key) {
            Some(serde_value) => {
                let value = serde_yaml::from_value(serde_value.clone())?;
                Ok(value)
            },
            None => Err(format!("key '{key}' not found in the instance").into()),
        }
    }
}

/// Aggregator stores Go callbacks for submissions
/// 
/// The check stores a pointer to the Aggregator structure declared in Cgo
#[repr(C)]
#[derive(Clone, Copy)]
pub struct Aggregator {
    cb_submit_metric: SubmitMetric,
    cb_submit_service_check: SubmitServiceCheck,
    cb_submit_event: SubmitEvent,
    cb_submit_histogram_bucket: SubmitHistogramBucket,
    cb_submit_event_platform_event: SubmitEventPlatformEvent,
}

impl Aggregator {
    pub fn from_raw(aggregator_ptr: *const Aggregator) -> Self {
        unsafe { *aggregator_ptr }
    }

    // TODO: optional arguements should use Option
    pub fn submit_metric(&self, check_id: &str, metric_type: MetricType, name: &str, value: f64, tags: &[String], hostname: &str, flush_first_value: bool) {
        // convert to C strings
        let cstr_check_id = to_cstring(check_id);
        let cstr_name = to_cstring(name);
        let cstr_tags = to_cstring_array(tags);
        let cstr_hostname = to_cstring(hostname);

        // submit the metric
        (self.cb_submit_metric)(
            cstr_check_id,
            metric_type,
            cstr_name,
            value,
            cstr_tags,
            cstr_hostname,
            flush_first_value,
        );

        // free every C string allocated
        free_cstring(cstr_check_id);
        free_cstring(cstr_name);
        free_cstring_array(cstr_tags);
        free_cstring(cstr_hostname);
    }

    pub fn submit_service_check(&self, check_id: &str, name: &str, status: i32, tags: &[String], hostname: &str, message: &str) {
        // convert to C strings
        let cstr_check_id = to_cstring(check_id);
        let cstr_name = to_cstring(name);
        let cstr_tags = to_cstring_array(tags);
        let cstr_hostname = to_cstring(hostname);
        let cstr_message = to_cstring(message);

        // submit the service check
        (self.cb_submit_service_check)(
            cstr_check_id,
            cstr_name,
            status,
            cstr_tags,
            cstr_hostname,
            cstr_message,
        );

        // free every C string allocated
        free_cstring(cstr_check_id);
        free_cstring(cstr_name);
        free_cstring_array(cstr_tags);
        free_cstring(cstr_hostname);
        free_cstring(cstr_message);

    }

    pub fn submit_event(&self, check_id: &str, event: &Event) {
        // convert to C strings
        let cstr_check_id = to_cstring(check_id);

        // submit the service check
        (self.cb_submit_event)(
            cstr_check_id,
            event,
        );

        // free every C string allocated
        free_cstring(cstr_check_id);
    }
    pub fn submit_histogram_bucket(&self, check_id: &str, metric_name: &str, value: c_longlong, lower_bound: f32, upper_bound: f32, monotonic: c_int, hostname: &str, tags: &[String], flush_first_value: bool) {
        // convert to C strings
        let cstr_check_id = to_cstring(check_id);
        let cstr_metric_name = to_cstring(metric_name);
        let cstr_hostname = to_cstring(hostname);
        let cstr_tags = to_cstring_array(tags);

        // submit the histogram bucket
        (self.cb_submit_histogram_bucket)(
            cstr_check_id,
            cstr_metric_name,
            value,
            lower_bound,
            upper_bound,
            monotonic,
            cstr_hostname,
            cstr_tags,
            flush_first_value,
        );

        // free every C string allocated
        free_cstring(cstr_check_id);
        free_cstring(cstr_metric_name);
        free_cstring(cstr_hostname);
        free_cstring_array(cstr_tags);
    }

    pub fn submit_event_platform_event(&self, check_id: &str, raw_event_pointer: &str, raw_event_size: c_int, event_type: &str) {
        // convert to C strings
        let cstr_check_id = to_cstring(check_id);
        let cstr_raw_event_pointer = to_cstring(raw_event_pointer);
        let cstr_event_type = to_cstring(event_type);

        // submit the event platform event
        (self.cb_submit_event_platform_event)(
            cstr_check_id,
            cstr_raw_event_pointer,
            raw_event_size,
            cstr_event_type,
        );

        // free every C string allocated
        free_cstring(cstr_check_id);
        free_cstring(cstr_raw_event_pointer);
        free_cstring(cstr_event_type);
    }
}