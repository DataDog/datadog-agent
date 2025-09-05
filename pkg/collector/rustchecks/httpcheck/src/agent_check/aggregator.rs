use serde_json::Value;
use serde::de::DeserializeOwned;

use super::helpers::*;

use std::collections::HashMap;
use std::ffi::{c_char, c_double, c_float, c_int, c_long, c_longlong};
use std::error::Error;

// replica of the Agent metric type enum
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

// replica of the Agent event struct
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

// signature of SubmitMetric
type SubmitMetric = extern "C" fn(
    *mut c_char,        // check id
    MetricType,         // metric type
    *mut c_char,        // name
    c_double,           // value
    *mut *mut c_char,   // tags
    *mut c_char,        // hostname
    bool,               // flush first value
);

// signature of SubmitServiceCheck
type SubmitServiceCheck = extern "C" fn(
    *mut c_char,        // check id
    *mut c_char,        // name
    c_int,              // status
    *mut *mut c_char,   // tags
    *mut c_char,        // hostname
    *mut c_char,        // message
);

// signature of SubmitEvent
type SubmitEvent = extern "C" fn(
    *mut c_char, // check_id
    Event,       // event
);
// signature of SubmitHistogramBucket
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

// signature of SubmitEventPlatformEvent
type SubmitEventPlatformEvent = extern "C" fn(
    *mut c_char, // check_id
    *mut c_char, // raw event pointer
    c_int,       // raw event size
    *mut c_char, // event type
);

#[repr(C)]
pub struct Instance {
    map: HashMap<String, Value>,
}

impl Instance {
    pub fn new(instance_str: &str) -> Result<Self, Box<dyn Error>> {
        let map: HashMap<String, Value> = serde_json::from_str(instance_str)?;
        let instance = Self { map };
        Ok(instance)
        
    }

    pub fn get<T>(&self, key: &str) -> Result<T, Box<dyn Error>>
    where 
        T: DeserializeOwned,
    {
        match self.map.get(key) {
            Some(serde_value) => {
                let value = serde_json::from_value(serde_value.clone())?;
                Ok(value)
            },
            None => Err(format!("key '{key}' not found in the instance").into()),
        }
    }
}

// Aggregator stores callbacks for submitting metrics, service checks...
#[repr(C)]
pub struct Aggregator {
    cb_submit_metric: SubmitMetric,
    cb_submit_service_check: SubmitServiceCheck,
    cb_submit_event: SubmitEvent,
    cb_submit_histogram_bucket: SubmitHistogramBucket,
    cb_submit_event_platform_event: SubmitEventPlatformEvent,
}

impl Aggregator {
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
}
