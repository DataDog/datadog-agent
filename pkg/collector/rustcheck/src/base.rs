use std::ffi::{c_char, c_double, CString};

// utility functions to convert Rust types to C-compatible types
pub fn to_cstring(string: &String) -> *mut c_char {
    CString::new(string.as_str()).expect("Can't cast Rust string to char*").into_raw()
}

pub fn to_cstring_array(vec: &[String]) -> *mut *mut c_char {
    let mut c_vec: Vec<*mut c_char> = vec.iter().map(|s| to_cstring(s)).collect();
    c_vec.push(std::ptr::null_mut()); // null-terminate the array

    let vec_ptr = c_vec.as_mut_ptr();
    std::mem::forget(c_vec);
    vec_ptr
}

// Replica of the Agent metric type enum
#[repr(C)]
#[derive(Clone, Copy)]
pub enum MetricType {
    Gauge = 0,
    Rate = 1,
    Count = 2,
    MonotonicCount = 3,
    Counter = 4,
    Histogram = 5,
    Historate = 6,
}

// this is the structure that is returned to RTLoader
// it contains the same fields as AgentCheck but in a C-compatible format
// later we could have a Rust Payload struct, and AgentCheck will have a Vec<Payload> field to send multiple metrics at once
#[repr(C)]
pub struct Payload {
    name: *mut c_char,
    metric_type: MetricType,
    value: c_double,
    tags: *mut *mut c_char,
    tags_length: usize,
    hostname: *mut c_char,
}

impl Payload {
    pub fn new(name: &String, metric_type: &MetricType, value: &f64, tags: &Vec<String>, hostname: &String) -> Self {
        Payload {
            name: to_cstring(name),
            metric_type: *metric_type,
            value: *value,
            tags: to_cstring_array(tags),
            tags_length: tags.len(),
            hostname: to_cstring(hostname),
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn Free(ptr: *mut Payload) {
    if !ptr.is_null() {
        unsafe { 
            drop(Box::from_raw(ptr)); // TODO: also drop fields inside Payload to avoid leaks
        }
    }
}

pub struct AgentCheck {
    name: String,
    metric_type: MetricType,
    value: f64,
    tags: Vec<String>,
    hostname: String,
}

impl AgentCheck {
    // default check constructor
    // default field values should be changed by any methods that create metric like gauge, rate, etc. before sending the payload
    pub fn new() -> Self {
        AgentCheck {
            name: String::from(""),
            metric_type: MetricType::Gauge,
            value: 0.0,
            tags: Vec::new(),
            hostname: String::from(""),
        }
    }

    // method used to modify fields beside hostname which is not meant to change
    // used by every methods that can send a payload
    fn set_metric_info(&mut self, name: String, metric_type: MetricType, value: f64, tags: Vec<String>, hostname: String) {
        self.name = name;
        self.metric_type = metric_type;
        self.value = value;
        self.tags = tags;
        self.hostname = hostname;
    }

    // TODO: use Into<String> trait to allow passing any type of string that can be converted to String ???
    pub fn gauge(&mut self, name: String, value: f64, tags: Vec<String>, hostname: String) {
        self.set_metric_info(name, MetricType::Gauge, value, tags, hostname);
    }

    pub fn send_payload(&self) -> *mut Payload {
        let payload = Payload::new( &self.name, &self.metric_type, &self.value, &self.tags, &self.hostname );
        Box::into_raw(Box::new(payload))
    }
}
