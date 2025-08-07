use super::helpers::*;

use std::ffi::{c_char, c_double, c_float, c_int, c_long, c_longlong, CStr};

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
pub struct CheckInstance {
    check_id: *mut c_char,
    cb_submit_metric: Option<SubmitMetric>,
    cb_submit_service_check: Option<SubmitServiceCheck>,
    cb_submit_event: Option<SubmitEvent>,
    cb_submit_histogram_bucket: Option<SubmitHistogramBucket>,
    cb_submit_event_platform_event: Option<SubmitEventPlatformEvent>,
    // ...
}

impl CheckInstance {
    pub fn get_check_id(&self) -> String {
        unsafe { CStr::from_ptr(self.check_id) }.to_str().unwrap().to_string()
    }

    pub fn get_callbacks(&self) -> Aggregator {
        if let (
                Some(cb_submit_metric),
                Some(cb_submit_service_check),
                Some(cb_submit_event),
                Some(cb_submit_histogram_bucket),
                Some(cb_submit_event_platform_event)
            ) = (self.cb_submit_metric, self.cb_submit_service_check, self.cb_submit_event, self.cb_submit_histogram_bucket, self.cb_submit_event_platform_event) {
            
            Aggregator {
                cb_submit_metric,
                cb_submit_service_check,
                cb_submit_event,
                cb_submit_histogram_bucket,
                cb_submit_event_platform_event,
            }
                
        } else {
            println!("Some callbacks are null, using default aggregator");
            Aggregator::default()
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

extern "C" fn dummy_submit_metric(
    _check_id: *mut c_char,
    _metric_type: MetricType,
    _name: *mut c_char,
    _value: c_double,
    _tags: *mut *mut c_char,
    _hostname: *mut c_char,
    _flush_first_value: bool,
) {}

extern "C" fn dummy_submit_service_check(
    _check_id: *mut c_char,
    _name: *mut c_char,
    _status: c_int,
    _tags: *mut *mut c_char,
    _hostname: *mut c_char,
    _message: *mut c_char,
) {}

extern "C" fn dummy_submit_event(
    _check_id: *mut c_char,
    _event: Event,
) {}

extern "C" fn dummy_submit_histogram_bucket(
    _check_id: *mut c_char,
    _metric_name: *mut c_char,
    _value: c_longlong,
    _lower_bound: c_float,
    _upper_bound: c_float,
    _monotonic: c_int,
    _hostname: *mut c_char,
    _tags: *mut *mut c_char,
    _flush_first_value: bool,
) {}

extern "C" fn dummy_submit_event_platform_event(
    _check_id: *mut c_char,
    _raw_event_ptr: *mut c_char,
    _raw_event_size: c_int,
    _event_type: *mut c_char,
) {}

impl Default for Aggregator {
    fn default() -> Self {
        Aggregator {
            cb_submit_metric: dummy_submit_metric,
            cb_submit_service_check: dummy_submit_service_check,
            cb_submit_event: dummy_submit_event,
            cb_submit_histogram_bucket: dummy_submit_histogram_bucket,
            cb_submit_event_platform_event: dummy_submit_event_platform_event,
        }
    }
}