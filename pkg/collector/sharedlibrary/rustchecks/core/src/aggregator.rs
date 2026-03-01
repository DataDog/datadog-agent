use crate::cstring::{CStringArrayGuard, CStringGuard};

use std::ffi::{c_char, c_double, c_float, c_int, c_long, c_longlong};

use anyhow::{Ok, Result};

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

/// Replica of the Agent service check status
#[repr(C)]
pub enum ServiceCheckStatus {
    OK = 0,
    WARNING = 1,
    CRITICAL = 2,
    UNKNOWN = 3,
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
    *mut c_char,      // check id
    MetricType,       // metric type
    *mut c_char,      // name
    c_double,         // value
    *mut *mut c_char, // tags
    *mut c_char,      // hostname
    bool,             // flush first value
);

/// Signature of the submit service check function
type SubmitServiceCheck = extern "C" fn(
    *mut c_char,      // check id
    *mut c_char,      // name
    c_int,            // status
    *mut *mut c_char, // tags
    *mut c_char,      // hostname
    *mut c_char,      // message
);

/// Signature of the submit event function
type SubmitEvent = extern "C" fn(
    *mut c_char,  // check_id
    *const Event, // event
);

/// Signature of the submit histogram bucket function
type SubmitHistogramBucket = extern "C" fn(
    *mut c_char,      // check_id
    *mut c_char,      // metric name
    c_longlong,       // value
    c_float,          // lower bound
    c_float,          // upper bound
    c_int,            // monotonic
    *mut c_char,      // hostname
    *mut *mut c_char, // tags
    bool,             // flush first value
);

/// Signature of the submit event platform event function
type SubmitEventPlatformEvent = extern "C" fn(
    *mut c_char, // check_id
    *mut c_char, // raw event pointer
    c_int,       // raw event size
    *mut c_char, // event type
);

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
    pub fn new(
        cb_submit_metric: SubmitMetric,
        cb_submit_service_check: SubmitServiceCheck,
        cb_submit_event: SubmitEvent,
        cb_submit_histogram_bucket: SubmitHistogramBucket,
        cb_submit_event_platform_event: SubmitEventPlatformEvent,
    ) -> Self {
        Self {
            cb_submit_metric,
            cb_submit_service_check,
            cb_submit_event,
            cb_submit_histogram_bucket,
            cb_submit_event_platform_event,
        }
    }

    pub fn from_ptr(ptr: *const Aggregator) -> Self {
        unsafe { *ptr }.clone()
    }

    pub fn submit_metric(
        &self,
        check_id: &str,
        metric_type: MetricType,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        // create C strings guards to automatically free the underlying C strings
        let cstr_check_id = CStringGuard::new(check_id)?;
        let cstr_name = CStringGuard::new(name)?;
        let cstr_tags = CStringArrayGuard::new(tags)?;
        let cstr_hostname = CStringGuard::new(hostname)?;

        // submit the metric
        (self.cb_submit_metric)(
            cstr_check_id.as_ptr(),
            metric_type,
            cstr_name.as_ptr(),
            value,
            cstr_tags.as_ptr(),
            cstr_hostname.as_ptr(),
            flush_first_value,
        );

        Ok(())
    }

    pub fn submit_service_check(
        &self,
        check_id: &str,
        name: &str,
        status: ServiceCheckStatus,
        tags: &[String],
        hostname: &str,
        message: &str,
    ) -> Result<()> {
        // create C strings guards to automatically free the underlying C strings
        let cstr_check_id = CStringGuard::new(check_id)?;
        let cstr_name = CStringGuard::new(name)?;
        let cstr_tags = CStringArrayGuard::new(tags)?;
        let cstr_hostname = CStringGuard::new(hostname)?;
        let cstr_message = CStringGuard::new(message)?;

        // submit the service check
        (self.cb_submit_service_check)(
            cstr_check_id.as_ptr(),
            cstr_name.as_ptr(),
            status as c_int,
            cstr_tags.as_ptr(),
            cstr_hostname.as_ptr(),
            cstr_message.as_ptr(),
        );

        Ok(())
    }
    pub fn submit_event(
        &self,
        check_id: &str,
        title: &str,
        text: &str,
        timestamp: c_long,
        priority: &str,
        host: &str,
        tags: &[String],
        alert_type: &str,
        aggregation_key: &str,
        source_type_name: &str,
        event_type: &str,
    ) -> Result<()> {
        // create C strings guards to automatically free the underlying C strings
        let cstr_check_id = CStringGuard::new(check_id)?;

        let cstr_title = CStringGuard::new(title)?;
        let cstr_text = CStringGuard::new(text)?;
        let cstr_priority = CStringGuard::new(priority)?;
        let cstr_host = CStringGuard::new(host)?;
        let cstr_tags = CStringArrayGuard::new(tags)?;
        let cstr_alert_type = CStringGuard::new(alert_type)?;
        let cstr_aggregation_key = CStringGuard::new(aggregation_key)?;
        let cstr_source_type_name = CStringGuard::new(source_type_name)?;
        let cstr_event_type = CStringGuard::new(event_type)?;

        let event = Event {
            title: cstr_title.as_ptr(),
            text: cstr_text.as_ptr(),
            timestamp,
            priority: cstr_priority.as_ptr(),
            host: cstr_host.as_ptr(),
            tags: cstr_tags.as_ptr(),
            alert_type: cstr_alert_type.as_ptr(),
            aggregation_key: cstr_aggregation_key.as_ptr(),
            source_type_name: cstr_source_type_name.as_ptr(),
            event_type: cstr_event_type.as_ptr(),
        };

        // submit the event
        (self.cb_submit_event)(cstr_check_id.as_ptr(), &event);

        Ok(())
    }

    pub fn submit_histogram_bucket(
        &self,
        check_id: &str,
        metric_name: &str,
        value: c_longlong,
        lower_bound: f32,
        upper_bound: f32,
        monotonic: c_int,
        hostname: &str,
        tags: &[String],
        flush_first_value: bool,
    ) -> Result<()> {
        // create C strings guards to automatically free the underlying C strings
        let cstr_check_id = CStringGuard::new(check_id)?;
        let cstr_metric_name = CStringGuard::new(metric_name)?;
        let cstr_hostname = CStringGuard::new(hostname)?;
        let cstr_tags = CStringArrayGuard::new(tags)?;

        // submit the histogram bucket
        (self.cb_submit_histogram_bucket)(
            cstr_check_id.as_ptr(),
            cstr_metric_name.as_ptr(),
            value,
            lower_bound,
            upper_bound,
            monotonic,
            cstr_hostname.as_ptr(),
            cstr_tags.as_ptr(),
            flush_first_value,
        );

        Ok(())
    }

    pub fn submit_event_platform_event(
        &self,
        check_id: &str,
        raw_event: &str,
        raw_event_size: c_int,
        event_type: &str,
    ) -> Result<()> {
        // create C strings guards to automatically free the underlying C strings
        let cstr_check_id = CStringGuard::new(check_id)?;
        let cstr_raw_event = CStringGuard::new(raw_event)?;
        let cstr_event_type = CStringGuard::new(event_type)?;

        // submit the event platform event
        (self.cb_submit_event_platform_event)(
            cstr_check_id.as_ptr(),
            cstr_raw_event.as_ptr(),
            raw_event_size,
            cstr_event_type.as_ptr(),
        );

        Ok(())
    }
}
