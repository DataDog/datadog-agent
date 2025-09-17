use super::agent_check::AgentCheck;
use super::aggregator::{Config, Aggregator};
use super::cstring::free_cstring;

use std::error::Error;
use std::ffi::{c_char, CString};

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
    let check = AgentCheck::new(check_id_str, init_config_str, instance_config_str, aggregator_ptr)?;

    // run the custom implementation
    check.check()
}

/// Free the error string
#[unsafe(no_mangle)]
pub extern "C" fn Free(run_error: *mut c_char) {
    free_cstring(run_error);
}
