mod utils;
use utils::base::{CheckID, AgentCheck};

use std::error::Error;
use serde::Deserialize;

#[derive(Deserialize)]
struct Ip {
    origin: String,
}

// function executed by RTLoader
// instead of passing CheckID, it will be more flexible to pass a struct that contains
// the same info as the 'instance' variable in Python checks.
// pub extern "C" fn Run(instance: Instance) {
//      let check_id = instance.get("check_id").expect("'check_id' not found");
//      ...
// }
#[unsafe(no_mangle)]
pub extern "C" fn Run(check_id: CheckID) {
    // create the check instance that will handle everything
    let mut check = AgentCheck::new(check_id);

    // run the custom implementation
    // NOTE: may later change the prints to logs
    match check.check() {
        Ok(_) => {
            println!("Check completed successfully.");
        }
        Err(e) => {
            eprintln!("Error when running check: {}", e);
        }
    }
}

// custom check implementation
impl AgentCheck {
    pub fn check(&mut self) -> Result<(), Box<dyn Error>> {
        /* check implementation goes here */

        let json: Ip = reqwest::blocking::get("http://httpbin.org/ip")?.json()?;
        println!("IP: {}", json.origin);
        Ok(())
    }
}
