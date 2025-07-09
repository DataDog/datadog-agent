mod metric;
use metric::{Metric, Payload};

// function executed by RTLoader
#[unsafe(no_mangle)]
pub extern "C" fn RunCheck() -> *mut Payload {
    let mut check = Metric::new();

    // run the custom implementation
    check.check();

    // create and send the metric paylaod to RTLoader
    check.send_payload()
}

// custom check implementation
impl Metric {
    pub fn check(&mut self) {
        /* check implementation goes here */
        
        let name = String::from("so.metric");
        let value = 3.14;
        let tags = vec![String::from("tag:long-description-of-rust-check"), String::from("tag2:another-very-long-description-used-for-testing")];
        let hostname = String::from("");

        self.gauge(name, value, tags, hostname);
    }
}
