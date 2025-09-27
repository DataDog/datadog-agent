// FFI for running checks

use rust_check_core::{AgentCheck, Aggregator};
use crate::CheckImplementation;

use std::error::Error;
use std::ffi::{c_char, CString};

// TODO: replace fii.rs files with macro rules defined in core crate to prevent code duplicates

/// Entrypoint of the check
#[unsafe(no_mangle)]
pub extern "C" fn Run(check_id_str: *const c_char, init_config_str: *const c_char, instance_config_str: *const c_char, aggregator_ptr: *const Aggregator) -> *mut c_char {
    match create_and_run_check(check_id_str, init_config_str, instance_config_str, aggregator_ptr) {
        Ok(()) => std::ptr::null_mut(),
        Err(e) => CString::new(e.to_string()).unwrap_or_default().into_raw(),
    }
}

/// Build the check structure and execute its custom implementation
fn create_and_run_check(check_id_str: *const c_char, init_config_str: *const c_char, instance_config_str: *const c_char, aggregator_ptr: *const Aggregator) -> Result<(), Box<dyn Error>> {
    // create the check instance
    let check = AgentCheck::from(check_id_str, init_config_str, instance_config_str, aggregator_ptr)?;

    // run the custom implementation
    check.check()
}
