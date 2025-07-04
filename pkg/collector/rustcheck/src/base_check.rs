use std::ffi::{c_char, c_double, CString};

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
// it contains the same fields as BaseCheck but in a C-compatible format
// later we could have a Rust Payload struct, and BaseCheck will have a Vec<Payload> field to send multiple metrics at once
#[repr(C)]
pub struct Payload {
    name: *mut c_char,
    metric_type: MetricType,
    value: c_double,
    tags: *mut *mut c_char,
    hostname: *mut c_char,
}

impl Payload {
    // tags not used yet because it's not used by RTLoader
    pub fn new(name: &String, metric_type: &MetricType, value: &f64, _tags: &Vec<String>, hostname: &String) -> Payload {
        Payload {
            name: CString::new(name.as_str()).unwrap().into_raw(),
            metric_type: *metric_type,
            value: *value,
            tags: std::ptr::null_mut(),
            hostname: CString::new(hostname.as_str()).unwrap().into_raw(),
        }        
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn FreePayload(ptr: *mut Payload) {
    if !ptr.is_null() {
        unsafe { 
            drop(Box::from_raw(ptr));
        }
    }
}

pub struct BaseCheck {
    name: String,
    metric_type: MetricType,
    value: f64,
    tags: Vec<String>,
    hostname: String,
}

impl BaseCheck {
    // default check constructor
    // default field values should be changed by any methods that create metric like gauge, rate, etc. before sending the payload
    pub fn new(hostname: &str) -> BaseCheck {
        BaseCheck {
            name: String::from(""),
            metric_type: MetricType::Gauge,
            value: 0.0,
            tags: Vec::new(),
            hostname: hostname.to_string(),
        }
    }

    // method used to modify fields beside hostname which is not meant to change
    // used by every methods that can send a payload
    fn set_metric_info(&mut self, name: String, metric_type: MetricType, value: f64, tags: Vec<String>) {
        self.name = name;
        self.metric_type = metric_type;
        self.value = value;
        self.tags = tags;
    }

    pub fn gauge(&mut self, name: &str, value: f64, tags: Vec<String>) {
        self.set_metric_info(name.to_string(), MetricType::Gauge, value, tags);
    }

    pub fn send_payload(&self) -> *mut Payload {
        let payload = Payload::new( &self.name, &self.metric_type, &self.value, &self.tags, &self.hostname );
        Box::into_raw(Box::new(payload))
    }
}
