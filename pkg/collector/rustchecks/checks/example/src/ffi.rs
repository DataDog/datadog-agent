// FFI for running checks

use rust_check_core::{AgentCheck, Aggregator, to_rust_string};
use crate::CheckImplementation;
use std::error::Error;
use std::ffi::{c_char, CString};

// TODO: replace ffi.rs files with macro rules defined in core crate to prevent code duplicates

/// Entrypoint of the check
#[unsafe(no_mangle)]
pub extern "C" fn Run(check_id_str: *mut c_char, init_config_str: *mut c_char, instance_config_str: *mut c_char, aggregator_ptr: *mut Aggregator) -> *mut c_char {
    match create_and_run_check(check_id_str, init_config_str, instance_config_str, aggregator_ptr) {
        Ok(()) => std::ptr::null_mut(),
        Err(e) => CString::new(e.to_string()).unwrap_or_default().into_raw(),
    }
}

/// Build the check structure and execute its custom implementation
fn create_and_run_check(check_id_str: *mut c_char, init_config_str: *mut c_char, instance_config_str: *mut c_char, aggregator_ptr: *mut Aggregator) -> Result<(), Box<dyn Error>> {
    // convert C args to Rust structs
    let check_id = to_rust_string(check_id_str)?;
    
    let init_config = to_rust_string(init_config_str)?;
    let instance_config = to_rust_string(instance_config_str)?;

    let aggregator = Aggregator::from_ptr(aggregator_ptr);

    // create the check instance
    let check = AgentCheck::new(check_id, init_config, instance_config, aggregator)?;

    // run the custom implementation
    check.check()
}
