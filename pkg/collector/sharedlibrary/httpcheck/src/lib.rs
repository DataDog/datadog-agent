mod utils;
use utils::base::AgentCheck;
use std::ffi::c_char;

// function executed by RTLoader
#[unsafe(no_mangle)]
pub extern "C" fn Run(check_id: *mut c_char) {
    // create the check instance that will handle everything
    let mut check = AgentCheck::new(check_id);

    // run the custom implementation
    check.check();
}

// custom check implementation
impl AgentCheck {
    pub fn check(&mut self) {
        /* check implementation goes here */

        /* http_check reimplementation based on the Python version */
        

    }
}
