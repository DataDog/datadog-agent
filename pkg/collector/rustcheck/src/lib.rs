mod metric;
use metric::{Metric, Payload};

// function executed by RTLoader
#[unsafe(no_mangle)]
pub extern "C" fn RunCheck() -> *mut Payload {
    let mut check = Metric::new("UNUSED"); // hostname field not used yet

    // run the custom implementation
    check.check();

    // create and send the metric paylaod to RTLoader
    check.send_payload()
}

// custom check implementation
impl Metric {
    pub fn check(&mut self) {
        /* check implementation goes here */

        self.gauge("so.metric", 3.14, vec!(String::from("tag:long-description-of-rust-check"), String::from("tag2:another-very-long-description-used-for-testing")));
    }
}
