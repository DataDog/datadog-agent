mod agent_check;
use agent_check::base::{Instance, AgentCheck, ServiceCheckStatus};

use std::error::Error;
use std::time::Instant;
use std::sync::Arc;
use std::net::{TcpStream, ToSocketAddrs};
use std::io::{ErrorKind, Read, Write};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use rustls::{KeyLogFile, ClientConnection, RootCertStore, Stream};
use rustls::pki_types::CertificateDer;
use webpki_roots::TLS_SERVER_ROOTS;
use x509_parser::parse_x509_certificate;

// entrypoint of the check
// instead of passing CheckID, it will be more flexible to pass a struct that contains
// the same info as the 'instance' variable in Python checks.
// pub extern "C" fn Run(instance: Instance) {
//      let check_id = instance.get("check_id").expect("'check_id' not found");
//      ...
// }
#[unsafe(no_mangle)]
pub extern "C" fn Run(instance: *const Instance) {
    // create the instance using the provided configuration
    let check = AgentCheck::new(instance);

    check.gauge("network.http.response_time", 1.0, &Vec::new(), "", false);
    println!("[SharedLibraryCheck] Check completed successfully.");

    // run the custom check implementation
    // TODO: change prints to logs
    // match check.check() {
    //     Ok(()) => {
    //         println!("[SharedLibraryCheck] Check completed successfully.");
    //     }
    //     Err(e) => {
    //         eprintln!("[SharedLibraryCheck] Error when running check: {e}");
    //     }
    // }
}

// custom check implementation
impl AgentCheck {
    pub fn check(self) -> Result<(), Box<dyn Error>> {
        /* check implementation goes here */

        // references for certificate expiration
        const DEFAULT_EXPIRE_DAYS_WARNING: i32 = 14;
        const DEFAULT_EXPIRE_DAYS_CRITICAL: i32 = 7;
        const DEFAULT_EXPIRE_WARNING: i32 = DEFAULT_EXPIRE_DAYS_WARNING * 24 * 3600;
        const DEFAULT_EXPIRE_CRITICAL: i32 = DEFAULT_EXPIRE_DAYS_CRITICAL * 24 * 3600;
                
        // hardcoded variables (should be passed as parameters)
        let url = "datadog.com";
        let mut tags = Vec::<String>::new();

        // ssl certs
        let mut peer_cert: Option<CertificateDer> = None;

        // list service checks and their custom tags
        let mut service_checks = Vec::<(String, ServiceCheckStatus, String)>::new();
        let service_checks_tags = tags.clone();

        // connection configuration
        let root_store = RootCertStore { roots: TLS_SERVER_ROOTS.into() };

        let mut config = rustls::ClientConfig::builder()
            .with_root_certificates(root_store)
            .with_no_client_auth();

        // Allow using SSLKEYLOGFILE.
        config.key_log = Arc::new(KeyLogFile::new());

        let server_name = url.try_into().unwrap();

        let mut conn = ClientConnection::new(Arc::new(config), server_name).unwrap();

        // establish communication
        let start_time = Instant::now();

        let addr = format!("{url}:443")
            .to_socket_addrs().unwrap()
            .next().unwrap();

        match TcpStream::connect_timeout(&addr, Duration::from_secs(5)) {
            Ok(mut sock) => {                
                // Create the TLS stream
                let mut tls = Stream::new(&mut conn, &mut sock);

                // http request header
                let request_header = format!(
                    "GET / HTTP/1.1\r\n\
                    Host: {url}\r\n\
                    Connection: close\r\n\
                    Accept-Encoding: identity\r\n\
                    \r\n"
                );

                // Send the HTTP request with the TLS stream
                let response_result = tls.write_all(request_header.as_bytes());
                let elapsed_time = start_time.elapsed();

                match response_result {
                    Ok(()) => {
                        // retrieve the first ssl certificate from the response
                        if let Some(certs) = tls.conn.peer_certificates() {
                            peer_cert = Some(certs[0].clone());
                        }

                        // add URL in tags list if not already present
                        let url_tag = format!("url:{url}");

                        if !tags.contains(&url_tag) {
                            tags.push(url_tag);
                        }

                        // submit response time metric
                        if service_checks.is_empty() {
                            self.gauge("network.http.response_time", elapsed_time.as_secs_f64(), &tags, "", false);
                        }

                        // read response from the TLS stream
                        let mut response_raw = Vec::new();

                        match tls.read_to_end(&mut response_raw) {
                            Ok(_) => {
                                let response = String::from_utf8(response_raw).unwrap();

                                // status code
                                let first_line = response.lines().nth(0).unwrap();
                                let status_code: u32 = first_line
                                    .split_whitespace()
                                    .nth(1).unwrap()
                                    .parse().unwrap();

                                // check for client or server http error
                                if status_code >= 400 {
                                    service_checks.push((
                                        "http.can_connect".to_string(),
                                        ServiceCheckStatus::CRITICAL,
                                        format!("Incorrect HTTP return code for url {url}. Expected 1xx or 2xx or 3xx, got {status_code}"),
                                    ));
                                } else {
                                    // TODO: content matching
                                    service_checks.push((
                                        "http.can_connect".to_string(),
                                        ServiceCheckStatus::OK,
                                        "UP".to_string(),
                                    ));
                                }
                            },
                            Err(e) => {
                                // NOTE: ErrorKind::WouldBlock is often linked to a timeout
                                // but not sure if we need to check for this error too
                                if e.kind() == ErrorKind::TimedOut || e.kind() == ErrorKind::WouldBlock {
                                    // timeout error
                                    service_checks.push((
                                        "http.can_connect".to_string(),
                                        ServiceCheckStatus::CRITICAL,
                                        format!("Timeout error: {e}. Connection failed after {} ms", elapsed_time.as_millis()),
                                    ));
                                } else {
                                    // connection error
                                    service_checks.push((
                                        "http.can_connect".to_string(),
                                        ServiceCheckStatus::CRITICAL,
                                        format!("Connection error: {e}. Connection failed after {} ms", elapsed_time.as_millis()),
                                    ));
                                }
                            },
                        }
                    },
                    Err(e) => {
                        // NOTE: ErrorKind::WouldBlock is often linked to a timeout
                        // but not sure if we need to check for this error too
                        if e.kind() == ErrorKind::TimedOut || e.kind() == ErrorKind::WouldBlock {
                            // timeout error
                            service_checks.push((
                                "http.can_connect".to_string(),
                                ServiceCheckStatus::CRITICAL,
                                format!("Timeout error: {e}. Connection failed after {} ms", elapsed_time.as_millis()),
                            ));
                        } else {
                            // connection error
                            service_checks.push((
                                "http.can_connect".to_string(),
                                ServiceCheckStatus::CRITICAL,
                                format!("Connection error: {e}. Connection failed after {} ms", elapsed_time.as_millis()),
                            ));
                        }
                    },
                }
            },
            Err(e) => {
                let elapsed_time = start_time.elapsed();

                // NOTE: ErrorKind::WouldBlock is often linked to a timeout
                // but not sure if we need to check for this error too
                if e.kind() == ErrorKind::TimedOut || e.kind() == ErrorKind::WouldBlock {
                    // timeout error
                    service_checks.push((
                        "http.can_connect".to_string(),
                        ServiceCheckStatus::CRITICAL,
                        format!("Timeout error: {e}. Connection failed after {} ms", elapsed_time.as_millis()),
                    ));
                } else {
                    // connection error
                    service_checks.push((
                        "http.can_connect".to_string(),
                        ServiceCheckStatus::CRITICAL,
                        format!("Connection error: {e}. Connection failed after {} ms", elapsed_time.as_millis()),
                    ));
                }
            },
        }

        // submit can connect metrics
        // (by looking at the above implementation, this if statement is useless because service_checks will always have at least one element)
        if !service_checks.is_empty() {
            // can connect metrics depend on the status of the first service check
            let (can_connect, cant_connect) = match service_checks[0].1 {
                ServiceCheckStatus::OK => (1.0, 0.0),
                _ => (0.0, 1.0),
            };

            self.gauge("network.http.can_connect", can_connect, &tags, "", false);
            self.gauge("network.http.cant_connect", cant_connect, &tags, "", true);
        }

        // get certificate expiration info
        let (status, days_left, seconds_left, msg) = match peer_cert {
            Some(cert) => inspect_cert(&cert),
            None => (
                ServiceCheckStatus::UNKNOWN,
                None,
                None,
                "Empty or no certificate found.".to_string(),
            ),
        };

        // submit ssl metrics if an expiration date was found
        if let Some(days_left) = days_left {
            self.gauge("http.ssl.days_left", days_left as f64, &tags, "", false);
        }

        if let Some(seconds_left) = seconds_left {
            self.gauge("http.ssl.seconds_left", seconds_left as f64, &tags, "", true);
        }

        // add ssl service check for certificate expiration
        service_checks.push((
            "http.ssl_cert".to_string(),
            status,
            msg,
        ));

        // submit every service check collected throughout the check
        for (sc_name, status, message) in service_checks {
            self.service_check(&sc_name, status, &service_checks_tags, "", &message);
        }

        return Ok(());

        // retrieve certificate expiration information and returns info for metric and service check
        fn inspect_cert(cert: &CertificateDer) -> (ServiceCheckStatus, Option<u64>, Option<u64>, String) {
            match parse_x509_certificate(cert) {
                Ok((_, x509)) => {
                    // get certificate remaining time before expiration
                    let expiration_timestamp = x509.validity().not_after.timestamp() as u64;
                    let current_timestamp = SystemTime::now()
                        .duration_since(UNIX_EPOCH)
                        .unwrap()
                        .as_secs();

                    let seconds_left = expiration_timestamp - current_timestamp;
                    let days_left = seconds_left / (24 * 3600);

                    // compare the time left before expiration to thresholds and return the corresponding service check
                    // TODO: check for several variables setup before taking the constants (need to pass an instance like in python to retrieve custom variables)
                    let seconds_warning = DEFAULT_EXPIRE_WARNING as u64;
                    let seconds_critical = DEFAULT_EXPIRE_CRITICAL as u64;

                    if seconds_left < seconds_critical {
                        (
                            ServiceCheckStatus::CRITICAL,
                            Some(days_left),
                            Some(seconds_left),
                            format!("This cert TTL is critical: only {days_left} days before it expires"),
                        )
                    } else if seconds_left < seconds_warning {
                        (
                            ServiceCheckStatus::WARNING,
                            Some(days_left),
                            Some(seconds_left),
                            format!("This cert is almost expired, only {days_left} days left"),
                        )
                    } else {
                        (
                            ServiceCheckStatus::OK,
                            Some(days_left),
                            Some(seconds_left),
                            format!("Days left: {days_left}"),
                        )
                    }
                }
                Err(e) => {
                    (
                        ServiceCheckStatus::UNKNOWN,
                        None,
                        None,
                        e.to_string(),
                    )
                },
            }
        }
    }
}
