use super::agent_check::AgentCheck;
use super::aggregator::{Instance, Aggregator};
use super::cstring::free_cstring;

use std::error::Error;
use std::ffi::{c_char, CString};

/// Entrypoint of the check
#[unsafe(no_mangle)]
pub extern "C" fn Run(instance_str: *const c_char, aggregator_ptr: *const Aggregator) -> *mut c_char {
    match run_check_impl(instance_str, aggregator_ptr) {
        Ok(()) => std::ptr::null_mut(),
        Err(e) => CString::new(e.to_string()).unwrap_or_default().into_raw(),        
    }
}

/// Free the error string
#[unsafe(no_mangle)]
pub extern "C" fn Free(run_error: *mut c_char) {
    free_cstring(run_error);
} 

/// Build the check structure and execute its custom implementation
fn run_check_impl(instance_str: *const c_char, aggregator_ptr: *const Aggregator) -> Result<(), Box<dyn Error>> {
    // from ffi arguments to Rust structure
    let instance = Instance::from_str(instance_str)?;
    let aggregator = Aggregator::from_raw(aggregator_ptr);

    // try to create the instance using the provided configuration
    let check = AgentCheck::new(instance, aggregator)?;

    // try to run its custom implementation
    check.check()
}
