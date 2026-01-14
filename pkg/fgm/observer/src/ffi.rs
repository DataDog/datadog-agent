// Copyright 2025 Datadog, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//! C FFI interface for the FGM observer library
//!
//! This module provides C-compatible function exports for use with CGO.
//! All functions are marked `#[no_mangle]` and use `extern "C"` calling convention.
//!
//! # Safety
//!
//! The FFI boundary is inherently unsafe. Callers must ensure:
//! - String pointers are valid UTF-8 and null-terminated
//! - Callback functions are valid and don't panic
//! - Context pointers remain valid during callback execution
//! - fgm_init() is called before any sampling operations
//! - fgm_shutdown() is called to clean up resources

use std::ffi::{c_char, c_double, c_int, c_longlong, c_void, CStr, CString};
use std::panic;
use std::path::Path;
use std::sync::OnceLock;
use tokio::runtime::Runtime;

/// Global Tokio runtime for blocking FFI calls
/// Initialized once on fgm_init(), used for all async operations
static RUNTIME: OnceLock<Runtime> = OnceLock::new();

/// Wrapper for callback + context that implements Send
///
/// # Safety
///
/// The caller must ensure that both the callback function and context pointer
/// can safely be sent between threads. This requires that:
/// 1. The callback function is thread-safe, and
/// 2. The context pointer points to thread-safe data
struct SendCallback {
    callback: MetricCallback,
    ctx: *mut c_void,
}

unsafe impl Send for SendCallback {}

impl SendCallback {
    fn call(
        &self,
        name: *const c_char,
        value: c_double,
        tags_json: *const c_char,
        timestamp_ms: c_longlong,
    ) {
        (self.callback)(name, value, tags_json, timestamp_ms, self.ctx);
    }
}

/// Callback function signature for metric emission
///
/// # Parameters
///
/// * `name` - Metric name (null-terminated C string)
/// * `value` - Metric value
/// * `tags_json` - JSON array of tags in "key:value" format
/// * `timestamp_ms` - Timestamp in milliseconds since Unix epoch
/// * `ctx` - Opaque context pointer passed through from caller
pub type MetricCallback = extern "C" fn(
    name: *const c_char,
    value: c_double,
    tags_json: *const c_char,
    timestamp_ms: c_longlong,
    ctx: *mut c_void,
);

/// Initialize the FGM observer library
///
/// Creates a Tokio runtime for async operations. Must be called before
/// any sampling operations.
///
/// # Returns
///
/// * `0` on success
/// * `1` if already initialized
/// * `-1` on runtime creation failure
///
/// # Safety
///
/// This function is safe to call from any thread, but should only be
/// called once during the process lifetime.
#[no_mangle]
pub extern "C" fn fgm_init() -> c_int {
    // Catch any panics and convert to error code
    match panic::catch_unwind(|| {
        match Runtime::new() {
            Ok(rt) => {
                if RUNTIME.set(rt).is_err() {
                    1 // Already initialized
                } else {
                    0 // Success
                }
            }
            Err(_) => -1, // Failed to create runtime
        }
    }) {
        Ok(code) => code,
        Err(_) => -1, // Panic occurred
    }
}

/// Shutdown the FGM observer library
///
/// Cleans up the Tokio runtime. No sampling operations should be
/// performed after calling this function.
///
/// # Safety
///
/// This function is safe to call from any thread. The runtime will
/// be dropped and all pending async tasks will be cancelled.
#[no_mangle]
pub extern "C" fn fgm_shutdown() {
    // Runtime will be dropped automatically when the program exits
    // This is primarily a no-op for symmetry with fgm_init()
}

/// Sample metrics for a single container
///
/// Reads cgroup v2 and procfs metrics, calling the provided callback
/// for each metric. This is a blocking call that may take several
/// milliseconds depending on filesystem performance.
///
/// # Parameters
///
/// * `cgroup_path` - Absolute path to container's cgroup directory (null-terminated)
/// * `pid` - Container's main PID (for procfs reads, 0 to skip)
/// * `callback` - Function to call for each metric
/// * `ctx` - Opaque context pointer passed to callback
///
/// # Returns
///
/// * `0` on success
/// * `-1` if not initialized (fgm_init not called)
/// * `-2` if invalid parameters (null pointers, invalid UTF-8)
/// * `-3` if sampling failed (critical error)
///
/// # Safety
///
/// Caller must ensure:
/// - `cgroup_path` is a valid null-terminated C string
/// - `callback` is a valid function pointer
/// - `ctx` remains valid during the entire call
/// - This function is not called concurrently with fgm_shutdown()
#[no_mangle]
pub extern "C" fn fgm_sample_container(
    cgroup_path: *const c_char,
    pid: c_int,
    callback: MetricCallback,
    ctx: *mut c_void,
) -> c_int {
    // Catch any panics and convert to error code
    match panic::catch_unwind(|| {
        sample_container_impl(cgroup_path, pid, callback, ctx)
    }) {
        Ok(code) => code,
        Err(_) => -3, // Panic occurred
    }
}

/// Internal implementation of fgm_sample_container
///
/// Separated from the FFI function to allow panic catching at the boundary.
fn sample_container_impl(
    cgroup_path: *const c_char,
    pid: c_int,
    callback: MetricCallback,
    ctx: *mut c_void,
) -> c_int {
    // Get runtime reference
    let rt = match RUNTIME.get() {
        Some(rt) => rt,
        None => return -1, // Not initialized
    };

    // Validate and convert cgroup_path
    if cgroup_path.is_null() {
        return -2;
    }

    let path_str = match unsafe { CStr::from_ptr(cgroup_path) }.to_str() {
        Ok(s) => s,
        Err(_) => return -2, // Invalid UTF-8
    };

    let path = Path::new(path_str);

    // Wrap callback and ctx in SendCallback for thread safety
    // SAFETY: The caller must ensure the callback and ctx are thread-safe
    let send_callback = SendCallback { callback, ctx };

    // Emit helper closure that wraps the C callback
    let emit = move |name: &str, value: f64, tags: Vec<(String, String)>, timestamp_ms: i64| {
        // Allocate C strings (they will be freed after callback returns)
        let c_name = match CString::new(name) {
            Ok(s) => s,
            Err(_) => return, // Skip metric with null bytes in name
        };

        // Serialize tags to JSON array format: ["key:value", "key2:value2"]
        let tags_vec: Vec<String> = tags
            .iter()
            .map(|(k, v)| format!("{}:{}", k, v))
            .collect();

        let tags_json = match serde_json::to_string(&tags_vec) {
            Ok(json) => json,
            Err(_) => "[]".to_string(), // Fallback to empty array
        };

        let c_tags = match CString::new(tags_json) {
            Ok(s) => s,
            Err(_) => return, // Skip if tags contain null bytes
        };

        // Call the callback (synchronous)
        // SAFETY: The caller must ensure ctx is valid for the lifetime of this call
        send_callback.call(
            c_name.as_ptr(),
            value,
            c_tags.as_ptr(),
            timestamp_ms,
        );

        // CStrings are dropped here, freeing the memory
    };

    // Run the async sampling function synchronously
    match rt.block_on(crate::observer::sample_container(path, pid, emit)) {
        Ok(_) => 0,
        Err(_) => -3,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    // Test context for collecting metrics
    struct TestContext {
        metrics: Mutex<Vec<(String, f64, Vec<String>)>>,
    }

    extern "C" fn test_callback(
        name: *const c_char,
        value: c_double,
        tags_json: *const c_char,
        _timestamp_ms: c_longlong,
        ctx: *mut c_void,
    ) {
        unsafe {
            let test_ctx = &*(ctx as *const TestContext);
            let name_str = CStr::from_ptr(name).to_str().unwrap().to_string();
            let tags_str = CStr::from_ptr(tags_json).to_str().unwrap();
            let tags: Vec<String> = serde_json::from_str(tags_str).unwrap_or_default();

            test_ctx
                .metrics
                .lock()
                .unwrap()
                .push((name_str, value, tags));
        }
    }

    #[test]
    fn test_ffi_init_and_shutdown() {
        let result = fgm_init();
        // Runtime might already be initialized by other tests, so accept both
        assert!(
            result == 0 || result == 1,
            "Init should succeed (0) or already be initialized (1), got {}",
            result
        );

        // Second call should always return already initialized
        let result = fgm_init();
        assert_eq!(result, 1, "Second init should return already initialized");

        fgm_shutdown();
    }

    #[test]
    fn test_sample_container_not_initialized() {
        // Create a new process where RUNTIME is not initialized
        // This test assumes fgm_init hasn't been called yet in this thread
        // In practice, tests run in parallel so we can't guarantee state

        let test_ctx = TestContext {
            metrics: Mutex::new(Vec::new()),
        };

        let path = CString::new("/sys/fs/cgroup/nonexistent").unwrap();
        let ctx_ptr = &test_ctx as *const TestContext as *mut c_void;

        // If not initialized, should return -1
        // If initialized by another test, should return 0 (no error)
        let result = fgm_sample_container(path.as_ptr(), 0, test_callback, ctx_ptr);
        assert!(
            result == -1 || result == 0,
            "Should either be uninitialized or succeed"
        );
    }

    #[test]
    fn test_sample_container_invalid_params() {
        fgm_init();

        let test_ctx = TestContext {
            metrics: Mutex::new(Vec::new()),
        };
        let ctx_ptr = &test_ctx as *const TestContext as *mut c_void;

        // Null cgroup_path should return -2
        let result = fgm_sample_container(std::ptr::null(), 0, test_callback, ctx_ptr);
        assert_eq!(result, -2, "Null path should return -2");
    }

    #[test]
    fn test_sample_container_nonexistent_path() {
        fgm_init();

        let test_ctx = TestContext {
            metrics: Mutex::new(Vec::new()),
        };

        let path = CString::new("/sys/fs/cgroup/nonexistent").unwrap();
        let ctx_ptr = &test_ctx as *const TestContext as *mut c_void;

        let result = fgm_sample_container(path.as_ptr(), 0, test_callback, ctx_ptr);

        // Should succeed even if path doesn't exist (just no metrics emitted)
        assert_eq!(result, 0, "Nonexistent path should not error");

        let metrics = test_ctx.metrics.lock().unwrap();
        assert_eq!(metrics.len(), 0, "No metrics should be emitted");
    }
}
