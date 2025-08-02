use std::ffi::{c_int, c_long, c_double, c_char, CString};

use libloading::{Library, library_filename, Symbol};

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

// signature of submit_metric in RTLoader
type SubmitMetricFn = extern "C" fn(
    *mut c_char,        // check_id
    MetricType,         // metric_type
    *mut c_char,        // name
    c_double,           // value
    *mut *mut c_char,   // tags
    *mut c_char,        // hostname
    bool,               // flush_first_value
);

// signature of submit_service_check in RTLoader
type SubmitServiceCheckFn = extern "C" fn(
    *mut c_char,        // check_id
    *mut c_char,        // name
    c_int,              // status
    *mut *mut c_char,   // tags
    *mut c_char,        // hostname
    *mut c_char,        // message
);

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

// signature of submit_event in RTLoader (not implemented yet in RTLoader)
type SubmitEventFn = extern "C" fn(
    *mut c_char, // check_id
    Event,       // event
);

pub fn to_cstring(string: &str) -> *mut c_char {
    CString::new(string).unwrap().into_raw()
}

pub fn to_cstring_array(vec: &[String]) -> *mut *mut c_char {
    let mut c_vec: Vec<*mut c_char> = vec.iter().map(|s| to_cstring(s)).collect();
    c_vec.push(std::ptr::null_mut()); // null-terminate the array

    let vec_ptr = c_vec.as_mut_ptr();
    std::mem::forget(c_vec); // prevent Rust from freeing the vector
    vec_ptr
}

fn free_cstring(ptr: *mut c_char) {
    unsafe { drop(CString::from_raw(ptr)) };
}

// should be used later to avoid memory leaks when rust types are converted to C types
fn free_cstring_array(ptr: *mut *mut c_char) {
    if ptr.is_null() {
        return;
    }

    let mut current = ptr;

    unsafe {   
        while !(*current).is_null() {
            drop(CString::from_raw(*current));
            current = current.add(1);
        }
    }
}

// Aggregator stores RTLoader symbols for submitting metrics, service checks.
pub struct Aggregator {
    lib: Library,
    cb_submit_metric: Symbol<'static, SubmitMetricFn>,
    cb_submit_service_check: Symbol<'static, SubmitServiceCheckFn>,
    // ...
}

impl Aggregator {
    pub fn new() -> Self {
        unsafe {
            let lib = Library::new(library_filename("datadog-agent-rtloader")).unwrap();
            let cb_submit_metric: Symbol<SubmitMetricFn> = lib.get(b"submit_metric").unwrap();
            let cb_submit_service_check: Symbol<SubmitServiceCheckFn> = lib.get(b"submit_service_check").unwrap();

            // we need to store the symbols in the struct so we need to have a longer lifetime
            let cb_submit_metric: Symbol<'static, SubmitMetricFn> = std::mem::transmute(cb_submit_metric);
            let cb_submit_service_check: Symbol<'static, SubmitServiceCheckFn> = std::mem::transmute(cb_submit_service_check);

            Aggregator {
                lib,
                cb_submit_metric,
                cb_submit_service_check,
            }
        }
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
}