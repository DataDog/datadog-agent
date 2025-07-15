mod utils;
use utils::base::{CheckID, AgentCheck};

use std::error::Error;

// function executed by RTLoader
#[unsafe(no_mangle)]
pub extern "C" fn Run(check_id: CheckID) {
    // create the check instance that will handle everything
    let mut check = AgentCheck::new(check_id);

    // run the custom implementation
    // NOTE: may later change the prints to logs
    match check.check() {
        Ok(_) => {
            println!("[SharedLibraryCheck] Check completed successfully.");
        }
        Err(e) => {
            eprintln!("[SharedLibraryCheck] Error when running check: {}", e);
        }
    }
}

// custom check implementation
impl AgentCheck {
    pub fn check(&mut self) -> Result<(), Box<dyn Error>> {
        /* check implementation goes here */

        let metric_name = "so.metric";
        let tags = vec![String::from("tag:long-description-of-rust-check"), String::from("tag2:another-very-long-description-used-for-testing")];

        self.gauge(metric_name, 3.14, &tags, "", false);

        let service_name = "so.service.check";
        let message = "Some service check message";

        self.service_check(service_name, 0, &tags, "", message);

        Ok(())
    }
}
