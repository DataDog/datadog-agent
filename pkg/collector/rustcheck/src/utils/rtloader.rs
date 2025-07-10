// load rtloader library at runtime

use libloading::{Library, library_filename, Symbol};
use std::ffi::{c_char, CString, c_double};

// Replica of the Agent metric type enum
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
    *mut c_char,
    MetricType,
    *mut c_char,
    c_double,
    *mut *mut c_char,
    *mut c_char,
    bool,
);

pub fn to_cstring(string: &String) -> *mut c_char {
    CString::new(string.as_str()).expect("Can't cast Rust string to char*").into_raw()
}

pub fn to_cstring_array(vec: &[String]) -> *mut *mut c_char {
    let mut c_vec: Vec<*mut c_char> = vec.iter().map(|s| to_cstring(s)).collect();
    c_vec.push(std::ptr::null_mut()); // null-terminate the array

    let vec_ptr = c_vec.as_mut_ptr();
    std::mem::forget(c_vec); // prevent Rust from freeing the vector before using it
    vec_ptr
}

// might be used later to avoid memory leaks when cast rust types to c types
fn _free_cstring_array(ptr: *mut *mut c_char) {
    if ptr.is_null() {
        return;
    }

    unsafe {
        let mut current = ptr;
        while !(*current).is_null() {
            drop(CString::from_raw(*current));
            current = current.add(1);
        }
        // Finally, free the array itself
        drop(Box::from_raw(ptr));
    }
}

pub fn submit_metric(check_id: &String, metric_type: MetricType, name: &String, value: f64, tags: &Vec<String>, hostname: &String, flush_first_value: bool) {
    unsafe {
        // might change the library name to an argument in the future
        // should be opened only once, since the symbol won't change
        let lib = Library::new(library_filename("datadog-agent-rtloader")).expect("Failed to load RTLoader library");
        let submit_metric: Symbol<SubmitMetricFn> = lib.get(b"submit_metric").expect("Failed to load RTLoader submit_metric function");

        submit_metric(
            to_cstring(check_id),
            metric_type,
            to_cstring(name),
            value,
            to_cstring_array(tags),
            to_cstring(hostname),
            flush_first_value,
        )
    }
}
