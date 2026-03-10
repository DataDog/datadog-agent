use crate::cstring::{CStringArrayGuard, CStringGuard};

use std::ffi::{c_char, c_double, c_float, c_int, c_longlong, c_void};
use std::os::raw::c_ulong;

use anyhow::{Ok, Result};

/// Log level constants matching ACR
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub enum LogLevel {
    Trace = 7,
    Debug = 10,
    Info = 20,
    Warning = 30,
    Error = 40,
    Critical = 50,
}

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

/// Event struct matching the ACR's C ABI layout.
/// All string fields use `*const c_char` and timestamp is `c_ulong`.
#[repr(C)]
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

// ---- Callback function pointer types matching ACR's callback module ----

/// (ctx, metric_type, metric_name, value, tags, hostname, flush_first_value)
type SubmitMetric = unsafe extern "C" fn(
    ctx: *mut c_void,
    metric_type: c_int,
    metric_name: *const c_char,
    value: c_double,
    tags: *const *const c_char,
    hostname: *const c_char,
    flush_first: c_int,
);

/// (ctx, sc_name, status, tags, hostname, message)
type SubmitServiceCheck = unsafe extern "C" fn(
    ctx: *mut c_void,
    name: *const c_char,
    status: c_int,
    tags: *const *const c_char,
    hostname: *const c_char,
    message: *const c_char,
);

/// (ctx, event)
type SubmitEvent = unsafe extern "C" fn(ctx: *mut c_void, event: *const Event);

/// (ctx, metric_name, value, lower_bound, upper_bound, monotonic, hostname, tags, flush_first_value)
type SubmitHistogram = unsafe extern "C" fn(
    ctx: *mut c_void,
    metric_name: *const c_char,
    value: c_longlong,
    lower_bound: c_float,
    upper_bound: c_float,
    monotonic: c_int,
    hostname: *const c_char,
    tags: *const *const c_char,
    flush_first: c_int,
);

/// (ctx, event, event_len, event_type)
type SubmitEventPlatformEvent = unsafe extern "C" fn(
    ctx: *mut c_void,
    event: *const c_char,
    event_len: c_int,
    event_type: *const c_char,
);

/// (ctx, level, message)
type SubmitLog = unsafe extern "C" fn(ctx: *mut c_void, level: c_int, message: *const c_char);

/// Callback struct matching the ACR's `callback::Callback` layout exactly.
///
/// The struct stores function pointers that the host (Go side) populates.
/// Each callback takes `ctx: *mut c_void` as its first argument, which the
/// host uses to route submissions to the correct sender.
#[repr(C)]
#[derive(Clone, Copy)]
pub struct Callback {
    pub submit_metric: SubmitMetric,
    pub submit_service_check: SubmitServiceCheck,
    pub submit_event: SubmitEvent,
    pub submit_histogram: SubmitHistogram,
    pub submit_event_platform_event: SubmitEventPlatformEvent,
    pub submit_log: SubmitLog,
}

/// CallbackContext bundles the callback function pointers with the opaque context
/// pointer, so that individual submit methods don't need to pass ctx explicitly.
pub struct CallbackContext {
    callback: Callback,
    ctx: *mut c_void,
}

// Safety: CallbackContext is only used within a single check invocation on one
// thread. The ctx pointer is owned by the host and valid for the duration of
// the call.
unsafe impl Send for CallbackContext {}
unsafe impl Sync for CallbackContext {}

impl CallbackContext {
    /// Create a new CallbackContext from a Callback pointer and opaque context.
    ///
    /// # Safety
    /// `callback_ptr` must point to a valid Callback struct for the duration
    /// of this CallbackContext's lifetime. `ctx` is an opaque pointer that
    /// will be passed through to each callback invocation.
    pub unsafe fn from_ptr(callback_ptr: *const Callback, ctx: *mut c_void) -> Self {
        Self {
            callback: unsafe { *callback_ptr },
            ctx,
        }
    }

    pub fn submit_metric(
        &self,
        metric_type: MetricType,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        let cstr_name = CStringGuard::new(name)?;
        let cstr_tags = CStringArrayGuard::new(tags)?;
        let cstr_hostname = CStringGuard::new(hostname)?;

        unsafe {
            (self.callback.submit_metric)(
                self.ctx,
                metric_type as c_int,
                cstr_name.as_ptr(),
                value,
                cstr_tags.as_ptr(),
                cstr_hostname.as_ptr(),
                flush_first_value as c_int,
            );
        }

        Ok(())
    }

    pub fn submit_service_check(
        &self,
        name: &str,
        status: ServiceCheckStatus,
        tags: &[String],
        hostname: &str,
        message: &str,
    ) -> Result<()> {
        let cstr_name = CStringGuard::new(name)?;
        let cstr_tags = CStringArrayGuard::new(tags)?;
        let cstr_hostname = CStringGuard::new(hostname)?;
        let cstr_message = CStringGuard::new(message)?;

        unsafe {
            (self.callback.submit_service_check)(
                self.ctx,
                cstr_name.as_ptr(),
                status as c_int,
                cstr_tags.as_ptr(),
                cstr_hostname.as_ptr(),
                cstr_message.as_ptr(),
            );
        }

        Ok(())
    }

    pub fn submit_event(
        &self,
        title: &str,
        text: &str,
        timestamp: c_ulong,
        priority: &str,
        host: &str,
        tags: &[String],
        alert_type: &str,
        aggregation_key: &str,
        source_type_name: &str,
        event_type: &str,
    ) -> Result<()> {
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
            ts: timestamp,
            priority: cstr_priority.as_ptr(),
            host: cstr_host.as_ptr(),
            tags: cstr_tags.as_ptr(),
            alert_type: cstr_alert_type.as_ptr(),
            aggregation_key: cstr_aggregation_key.as_ptr(),
            source_type_name: cstr_source_type_name.as_ptr(),
            event_type: cstr_event_type.as_ptr(),
        };

        unsafe {
            (self.callback.submit_event)(self.ctx, &event);
        }

        Ok(())
    }

    pub fn submit_histogram_bucket(
        &self,
        metric_name: &str,
        value: c_longlong,
        lower_bound: f32,
        upper_bound: f32,
        monotonic: c_int,
        hostname: &str,
        tags: &[String],
        flush_first_value: bool,
    ) -> Result<()> {
        let cstr_metric_name = CStringGuard::new(metric_name)?;
        let cstr_hostname = CStringGuard::new(hostname)?;
        let cstr_tags = CStringArrayGuard::new(tags)?;

        unsafe {
            (self.callback.submit_histogram)(
                self.ctx,
                cstr_metric_name.as_ptr(),
                value,
                lower_bound,
                upper_bound,
                monotonic,
                cstr_hostname.as_ptr(),
                cstr_tags.as_ptr(),
                flush_first_value as c_int,
            );
        }

        Ok(())
    }

    pub fn submit_event_platform_event(
        &self,
        raw_event: &str,
        raw_event_size: c_int,
        event_type: &str,
    ) -> Result<()> {
        let cstr_raw_event = CStringGuard::new(raw_event)?;
        let cstr_event_type = CStringGuard::new(event_type)?;

        unsafe {
            (self.callback.submit_event_platform_event)(
                self.ctx,
                cstr_raw_event.as_ptr(),
                raw_event_size,
                cstr_event_type.as_ptr(),
            );
        }

        Ok(())
    }

    pub fn submit_log(&self, level: LogLevel, message: &str) -> Result<()> {
        let cstr_message = CStringGuard::new(message)?;

        unsafe {
            (self.callback.submit_log)(self.ctx, level as c_int, cstr_message.as_ptr());
        }

        Ok(())
    }
}
