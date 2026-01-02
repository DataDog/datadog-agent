use crate::Result;
use crate::check::{HttpCheck, config};
use crate::sink::{Sink, event, event_platform_event, histogram, log, metric, service_check};

use anyhow::Context;
use libc::{c_char, c_double, c_float, c_int, c_longlong, c_ulong};
use std::collections::HashMap;
use std::ffi::CString;

pub struct Event {
    pub title: *const c_char,
    pub text: *const c_char,
    pub ts: c_ulong,
    pub priority: *const c_char,
    pub host: *const c_char,
    pub tags: *const *const c_char,
    pub alert_type: *const c_char,
    pub aggregation_key: *const c_char,
    pub source_type_name: *const c_char,
    pub event_type: *const c_char,
}

pub mod callback {
    use super::*;

    // (id, metric_type, metric_name, value, tags, hostname, flush_first_value)
    // void (*cb_submit_metric_t)(char *, metric_type_t, char *, double, char **, char *, bool)
    pub type SubmitMetric = unsafe extern "C" fn(
        id: *const c_char,
        metric_type: c_int,
        metric_name: *const c_char,
        value: c_double,
        tags: *const *const c_char,
        hostname: *const c_char,
        flush_first: c_int,
    );

    // (id, sc_name, status, tags, hostname, message)
    // void (*cb_submit_service_check_t)(char *, char *, int, char **, char *, char *)
    pub type SubmitServiceCheck = unsafe extern "C" fn(
        id: *const c_char,
        name: *const c_char,
        status: c_int,
        tags: *const *const c_char,
        hostname: *const c_char,
        message: *const c_char,
    );

    // (id, event)
    // void (*cb_submit_event_t)(char *, event_t *)
    pub type SubmitEvent = unsafe extern "C" fn(id: *const c_char, event: *const Event);

    // (id, metric_name, value, lower_bound, upper_bound, monotonic, hostname, tags, flush_first_value)
    // void (*cb_submit_histogram_bucket_t)(char *, char *, long long, float, float, int, char *, char **, bool)
    pub type SubmitHistogram = unsafe extern "C" fn(
        id: *const c_char,
        metric_name: *const c_char,
        value: c_longlong,
        lower_bound: c_float,
        upper_bound: c_float,
        monotonic: c_int,
        hostname: *const c_char,
        tags: *const *const c_char,
        flush_first: c_int,
    );

    // (id, event, event_type)
    // void (*cb_submit_event_platform_event_t)(char *, char *, int, char *)
    pub type SubmitEventPlatformEvent = unsafe extern "C" fn(
        id: *const c_char,
        event: *const c_char,
        event_len: c_int,
        event_type: *const c_char,
    );

    // rtloader/include/rtloader_types.h
    // pkg/collector/aggregator/aggregator.go
    #[repr(C)]
    #[derive(Debug, Copy, Clone)]
    pub struct Callback {
        pub submit_metric: SubmitMetric,
        pub submit_service_check: SubmitServiceCheck,
        pub submit_event: SubmitEvent,
        pub submit_histogram: SubmitHistogram,
        pub submit_event_platform_event: SubmitEventPlatformEvent,
    }
}

// TODO: try to avoid reallocating tags C array
pub struct SharedLibrary<'a> {
    callback: &'a callback::Callback,
}

impl<'a> SharedLibrary<'a> {
    pub fn new(callback: &'a callback::Callback) -> Self {
        SharedLibrary { callback }
    }
}

impl Sink for SharedLibrary<'_> {
    fn submit_metric(&self, metric: metric::Metric, flush_first: bool) -> Result<()> {
        let id = to_cstring(metric.id);
        let name = to_cstring(metric.name);
        let hostname = to_cstring(metric.hostname);
        let tags = map_to_cstring_array(&metric.tags);
        let tags = to_cchar_array(&tags);

        unsafe {
            (self.callback.submit_metric)(
                id.as_ptr(),
                metric.metric_type as c_int,
                name.as_ptr(),
                metric.value,
                tags.as_ptr(),
                hostname.as_ptr(),
                flush_first as c_int,
            );
        }
        Ok(())
    }

    fn submit_service_check(&self, service_check: service_check::ServiceCheck) -> Result<()> {
        let id = to_cstring(service_check.id);
        let name = to_cstring(service_check.name);
        let hostname = to_cstring(service_check.hostname);
        let message = to_cstring(service_check.message);
        let tags = map_to_cstring_array(&service_check.tags);
        let tags = to_cchar_array(&tags);

        unsafe {
            (self.callback.submit_service_check)(
                id.as_ptr(),
                name.as_ptr(),
                service_check.status as c_int,
                tags.as_ptr(),
                hostname.as_ptr(),
                message.as_ptr(),
            );
        }
        Ok(())
    }

    fn submit_event(&self, check_id: &str, event: event::Event) -> Result<()> {
        let id = to_cstring(check_id);
        let title = to_cstring(event.title);
        let text = to_cstring(event.text);
        let priority = to_cstring(event.priority);
        let hostname = to_cstring(event.hostname);
        let alert_type = to_cstring(event.alert_type);
        let aggregation_key = to_cstring(event.aggregation_key);
        let source_type_name = to_cstring(event.source_type_name);
        let event_type = to_cstring(event.event_type);
        let tags = map_to_cstring_array(&event.tags);
        let tags = to_cchar_array(&tags);

        let c_event = Event {
            title: title.as_ptr(),
            text: text.as_ptr(),
            ts: event.timestamp,
            priority: priority.as_ptr(),
            host: hostname.as_ptr(),
            tags: tags.as_ptr(),
            alert_type: alert_type.as_ptr(),
            aggregation_key: aggregation_key.as_ptr(),
            source_type_name: source_type_name.as_ptr(),
            event_type: event_type.as_ptr(),
        };

        unsafe {
            (self.callback.submit_event)(id.as_ptr(), &c_event);
        }
        Ok(())
    }

    fn submit_histogram(&self, histogram: histogram::Histrogram, flush_first: bool) -> Result<()> {
        let id = to_cstring(histogram.id);
        let metric_name = to_cstring(histogram.metric_name);
        let hostname = to_cstring(histogram.hostname);
        let tags = map_to_cstring_array(&histogram.tags);
        let tags = to_cchar_array(&tags);

        unsafe {
            (self.callback.submit_histogram)(
                id.as_ptr(),
                metric_name.as_ptr(),
                histogram.value as c_longlong,
                histogram.lower_bound as c_float,
                histogram.upper_bound as c_float,
                histogram.monotonic as c_int,
                hostname.as_ptr(),
                tags.as_ptr(),
                flush_first as c_int,
            );
        }
        Ok(())
    }

    fn submit_event_platform_event(&self, event: event_platform_event::Event) -> Result<()> {
        let id = to_cstring(event.id);
        let event_data = to_cstring(event.event);
        let event_type = to_cstring(event.event_type);

        unsafe {
            (self.callback.submit_event_platform_event)(
                id.as_ptr(),
                event_data.as_ptr(),
                event_data.as_bytes().len() as c_int,
                event_type.as_ptr(),
            );
        }
        Ok(())
    }

    fn log(&self, level: log::Level, message: String) {
        println!("[{level:?}] {message}")
    }
}

#[tokio::main(flavor = "current_thread")]
async fn async_do_check(
    http_check: &mut HttpCheck<SharedLibrary>
) {
    http_check.check().await
}

pub fn run(
    callback: &callback::Callback,
    check_id: &str,
    init_config: &str,
    instance_config: &str,
) -> Result<()> {
    let init_config: config::Init =
        serde_yaml::from_str(init_config).with_context(|| "Failed to parse init configuration")?;
    let instance_config: config::Instance = serde_yaml::from_str(instance_config)
        .with_context(|| "Failed to pars instance configuration")?;

    let shlib = SharedLibrary::new(callback);
    let mut http_check = HttpCheck::new(&shlib, check_id.to_string(), init_config, instance_config);
    async_do_check(&mut http_check);
    Ok(())
}

fn to_cstring<S>(str: S) -> CString
where
    S: AsRef<str>,
{
    CString::new(str.as_ref()).expect("bad alloc")
}

// fn to_cstring_array(arr: &Vec<String>) -> Vec<CString> {
//     arr.iter().map(to_cstring).collect()
// }

// FIXME name; trait?
fn map_to_cstring_array(map: &HashMap<String, String>) -> Vec<CString> {
    map.iter()
        .map(|(k, v)| to_cstring(format!("{}:{}", k, v)))
        .collect()
}

fn to_cchar_array(arr: &Vec<CString>) -> Vec<*const c_char> {
    arr.iter()
        .map(|s| s.as_ptr())
        .chain(std::iter::once(std::ptr::null()))
        .collect()
}

fn str_from_ptr<'a>(ptr: *const c_char) -> Option<&'a str> {
    if ptr == std::ptr::null() {
        return None;
    };
    unsafe { CStr::from_ptr(ptr) }.to_str().ok()
}

fn run_impl(
    callback: *const callback::Callback,
    check_id: *const c_char,
    init_config: *const c_char,
    instance_config: *const c_char,
) -> Result<()> {
    let callback = unsafe { &*callback }; // FIXME check for nullptr
    let check_id = str_from_ptr(check_id).ok_or(anyhow!("invalid check_id"))?;
    let init_config = str_from_ptr(init_config).ok_or(anyhow!("invalid init_config"))?;
    let instance_config =
        str_from_ptr(instance_config).ok_or(anyhow!("invalid instance_config"))?;

    shlib::run(callback, check_id, init_config, instance_config)
}

//(char *, char *, char *, const aggregator_t *, const char **);
#[unsafe(no_mangle)]
pub extern "C" fn Run(
    check_id: *const c_char,
    init_config: *const c_char,
    instance_config: *const c_char,
    callback: *const callback::Callback,
    error: *mut *const c_char,
) {
    let res = run_impl(callback, check_id, init_config, instance_config);
    if let Err(err) = res {
        println!("Oopsie: {}", err.to_string());
        unsafe {
            *error = CString::from_str(err.to_string().as_str())
                .expect("allocation error")
                .into_raw()
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn Version(_error: *mut *const c_char) -> *const c_char {
    http_check::version::VERSION.as_ptr().cast()
}
