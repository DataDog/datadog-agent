mod utils;
use utils::base::{CheckID, AgentCheck, ServiceCheckStatus};

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
    let check = AgentCheck::new(check_id);

    // run the custom implementation
    // TODO: change prints to logs
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
    pub fn check(self) -> Result<(), Box<dyn Error>> {
        /* check implementation goes here */

        fn fetch(url: &str) -> (Result<reqwest::blocking::Response, reqwest::Error>, std::time::Duration) {
            let client = reqwest::blocking::Client::new().get(url);

            let start = Instant::now();
            let res = client.send();
            let duration = start.elapsed();

            (res, duration)
        }

        // TODO:
        // - tags list
        // - errors handling
        // - servive check list (name, status, message)
        // - service check tags list
        // - ssl metrics

        // hardcoded variables (should be passed as parameters in an instance)
        let url = "https://datadoghq.com";
        let reponse_time = true;
        let ssl_expire = true;
        let uri_scheme = "https";

        let mut tags = Vec::<String>::new();

        // variables
        let mut service_checks: Vec<(String, ServiceCheckStatus, String)> = Vec::new();
        let service_tags: Vec<String> = Vec::new(); // to be defined by the tags


        // fetch the URL and measure the response time
        let (response, duration) = fetch(url);

        // check fetch result
        match response {
            Ok(resp) => {
                // add url in tags list if not already present
                let url_tag = format!("url:{}", url);

                if !tags.contains(&url_tag) {
                    tags.push(url_tag);
                }

                // response time metric if enabled
                if reponse_time {
                    self.gauge("network.http.response_time", duration.as_secs_f64(), &tags, "", false);
                }

                // check if http response status code corresponds to an error
                if resp.status().is_client_error() || resp.status().is_server_error() {
                    service_checks.push((
                        "http.can_connect".to_string(),
                        ServiceCheckStatus::CRITICAL,
                        format!("Incorrect HTTP return code for url {}. Expected 1xx or 2xx or 3xx, got {}", url, resp.status()),
                    ));
            
                // host is UP
                } else {
                    // TODO: content matching
                    service_checks.push((
                        "http.can_connect".to_string(),
                        ServiceCheckStatus::OK,
                        "UP".to_string(),
                    ));
                }
            }
            Err(e) => {
                if e.is_timeout() {
                    service_checks.push((
                        "http.can_connect".to_string(),
                        ServiceCheckStatus::CRITICAL,
                        format!("Timeout error: {}. Connection failed after {} ms", e.to_string(), duration.as_millis()),
                    ));

                } else if e.is_connect() {
                    service_checks.push((
                        "http.can_connect".to_string(),
                        ServiceCheckStatus::CRITICAL,
                        format!("Connection error: {}. Connection failed after {} ms", e.to_string(), duration.as_millis()),
                    ));
                
                } else {
                    service_checks.push((
                        "http.can_connect".to_string(),
                        ServiceCheckStatus::CRITICAL,
                        format!("Unhandled error: {}.", e.to_string()),
                    ));
                }
            }
        }

        // can connect metrics
        // (by looking at the above implementation, this if statement is useless)
        if !service_checks.is_empty() {
            let (can_connect, cant_connect) = match service_checks[0].1 {
                ServiceCheckStatus::OK => (1.0, 0.0),
                _ => (0.0, 1.0),
            };

            self.gauge("network.http.can_connect", can_connect, &tags, "", false);
            self.gauge("network.http.cant_connect", cant_connect, &tags, "", true);
        }

        // handle ssl certificate expiration
        if ssl_expire && uri_scheme == "https" {
            // retrieve ssl info (to be done)
            let status: ServiceCheckStatus = ServiceCheckStatus::OK;
            let msg: String = String::new();

            let days_left: f64 = 0.0;
            let seconds_left: f64 = 0.0;

            // ssl metrics
            self.gauge("http.ssl.days_left", days_left, &tags, "", false);
            self.gauge("http.ssl.seconds_left", seconds_left, &tags, "", true);

            // ssl service check
            service_checks.push((
                "http.ssl_cert".to_string(),
                status,
                msg,
            ));
        }

        // service checks
        for (sc_name, status, message) in service_checks {
            self.service_check(&sc_name, status, &service_tags, "", &message);
        }
        
        Ok(())
    }
}
