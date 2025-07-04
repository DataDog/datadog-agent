mod base_check;
use base_check::{BaseCheck, Payload};

// function run by RTLoader
#[unsafe(no_mangle)]
pub extern "C" fn RunCheck() -> *mut Payload {
    let mut check = BaseCheck::new("UNUSED");

    // run the custom implementation
    check.check();

    // create and send the metric paylaod to RTLoader
    check.send_payload()
}

impl BaseCheck {
    pub fn check(&mut self) {
        /* check implementation goes here */

        self.gauge("so.metric", 3.14, vec!(String::from("tag:test"), String::from("tag2:another-test")));
    }
}
