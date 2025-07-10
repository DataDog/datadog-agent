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

        let name = String::from("so.metric");
        let value = 3.14;
        let tags = vec![String::from("tag:long-description-of-rust-check"), String::from("tag2:another-very-long-description-used-for-testing")];
        let hostname = String::from("");

        self.gauge(name, value, tags, hostname, false);
    }
}
