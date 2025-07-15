mod utils;
use utils::base::{CheckID, AgentCheck};

use std::error::Error;

use std::time::Instant;

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

        // hardcoded variables (should be passed as parameters in an instance)
        let url = "https://datadoghq.com";
        let tags = Vec::<String>::new();

        let start = Instant::now();
        let response = reqwest::blocking::get(url)?;
        let duration = start.elapsed();

        if !response.status().is_success() {
            return Err(format!("Failed to fetch {}: {}", url, response.status()).into());
        }

        self.gauge("network.http.response_time", duration.as_secs_f64(), &tags, "", false);

        Ok(())
    }
}
