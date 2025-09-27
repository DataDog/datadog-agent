mod ffi;
use rust_check_core::{AgentCheck, ServiceCheckStatus};

use std::error::Error;

use std::time::Instant;
use std::sync::Arc;
use std::net::{TcpStream, ToSocketAddrs};
use std::io::{ErrorKind, Read, Write};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use rustls::{KeyLogFile, ClientConnection, RootCertStore, Stream};
use rustls::pki_types::{CertificateDer, ServerName};
use webpki_roots::TLS_SERVER_ROOTS;
use x509_parser::parse_x509_certificate;

/// Parsed URL
#[derive(Debug)]
pub struct Url {
    pub scheme: Option<String>,
    pub path: String,
    pub port: Option<String>,
}

impl Url {
    pub fn from(url: &str) -> Self {
        let mut remaining = url;
        
        // Extract scheme
        let scheme = if let Some(scheme_end) = remaining.find("://") {
            let extracted_scheme = remaining[..scheme_end].to_string();
            remaining = &remaining[scheme_end + 3..]; // Skip past "://"
            Some(extracted_scheme)
        } else {
            None
        };

        // Extract path and port
        let (path, port) = if let Some(port_start) = remaining.find(':') {
            let path = &remaining[..port_start];
            let port = &remaining[port_start + 1..];
            (path.to_string(), Some(port.to_string()))
        } else {
            (remaining.to_string(), None)
        };

        Self { scheme, path, port }
    }
}

pub trait CheckImplementation {
    fn check(&self) -> Result<(), Box<dyn Error>>;
}

impl CheckImplementation for AgentCheck {
    /// Check implementation
    fn check(&self) -> Result<(), Box<dyn Error>> {
        // references for certificate expiration
        const DEFAULT_EXPIRE_DAYS_WARNING: i32 = 14;
        const DEFAULT_EXPIRE_DAYS_CRITICAL: i32 = 7;
        const DEFAULT_EXPIRE_WARNING: i32 = DEFAULT_EXPIRE_DAYS_WARNING * 24 * 3600;
        const DEFAULT_EXPIRE_CRITICAL: i32 = DEFAULT_EXPIRE_DAYS_CRITICAL * 24 * 3600;

        // ssl certificates can be collected during the execution of the check
        // if not, a service check will be sent
        let mut peer_cert: Option<CertificateDer> = None;

        // parse the url given by the configuration
        let full_url_str: String = self.instance.get("url")?;
        let url = Url::from(&full_url_str);
        
        let mut service_checks = Vec::<(String, ServiceCheckStatus, String)>::new();
        
        let mut tags: Vec<String> = vec![];
        let service_checks_tags = tags.clone();

        // connection configuration
        let root_store = RootCertStore { roots: TLS_SERVER_ROOTS.into() };

        let mut config = rustls::ClientConfig::builder()
            .with_root_certificates(root_store)
            .with_no_client_auth();

        // Allow using SSLKEYLOGFILE
        config.key_log = Arc::new(KeyLogFile::new());

        let server_name = ServerName::try_from(url.path.clone())?.to_owned();

        let mut conn = ClientConnection::new(Arc::new(config), server_name)?;

        // establish communication
        let start_time = Instant::now();

        // set the port to 443 if none were given
        let addr = format!("{}:{}", url.path.clone(), url.port.unwrap_or("443".to_string()))
            .to_socket_addrs()?
            .next().unwrap();

        match TcpStream::connect_timeout(&addr, Duration::from_secs(5)) {
            Ok(mut sock) => {                
                // Create the TLS stream
                let mut stream = Stream::new(&mut conn, &mut sock);

                // http request header
                let request_header = format!(
                    "GET / HTTP/1.1\r\n\
                    Host: {}\r\n\
                    Connection: close\r\n\
                    Accept-Encoding: identity\r\n\
                    \r\n",
                    url.path
                );

                // Send the HTTP request with the TLS stream
                let response_result = stream.write_all(request_header.as_bytes());
                let elapsed_time = start_time.elapsed();

                match response_result {
                    Ok(()) => {
                        // retrieve the first ssl certificate from the response
                        if let Some(certs) = stream.conn.peer_certificates() {
                            peer_cert = Some(certs[0].clone());
                        }

                        // add URL in tags list if not already present
                        let url_tag = format!("url:{}", url.path);

                        if !tags.contains(&url_tag) {
                            tags.push(url_tag);
                        }

                        // submit response time metric
                        if service_checks.is_empty() {
                            self.gauge("network.http.response_time", elapsed_time.as_secs_f64(), &tags, "", false);
                        }

                        // read response from the TLS stream
                        let mut response_raw = Vec::new();

                        match stream.read_to_end(&mut response_raw) {
                            Ok(_) => {
                                let response = String::from_utf8(response_raw)?;

                                // status code
                                let first_line = response.lines().nth(0).unwrap();
                                let status_code: u32 = first_line
                                    .split_whitespace()
                                    .nth(1).unwrap()
                                    .parse()?;

                                // check for client or server http error
                                if status_code >= 400 {
                                    service_checks.push((
                                        "http.can_connect".to_string(),
                                        ServiceCheckStatus::CRITICAL,
                                        format!("Incorrect HTTP return code for url {}. Expected 1xx or 2xx or 3xx, got {}", url.path, status_code),
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

                // NOTE: ErrorKind::WouldBlock is often associated with a timeout
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

#[cfg(test)]
mod test {
    use super::*;

    #[test]
    fn test_url_parsing() {
        // Test full URL with scheme, hostname, and port
        let url1 = Url::from("https://localhost:5000");
        assert_eq!(url1.scheme, Some("https".to_string()));
        assert_eq!(url1.path, "localhost");
        assert_eq!(url1.port, Some("5000".to_string()));

        // Test URL without port
        let url2 = Url::from("http://example.com");
        assert_eq!(url2.scheme, Some("http".to_string()));
        assert_eq!(url2.path, "example.com");
        assert_eq!(url2.port, None);

        // Test hostname with port but no scheme
        let url3 = Url::from("localhost:8080");
        assert_eq!(url3.scheme, None);
        assert_eq!(url3.path, "localhost");
        assert_eq!(url3.port, Some("8080".to_string()));

        // Test just hostname
        let url4 = Url::from("example.com");
        assert_eq!(url4.scheme, None);
        assert_eq!(url4.path, "example.com");
        assert_eq!(url4.port, None);
    }

    #[test]
    fn test_check_implementation() -> Result<(), Box<dyn Error>> {
        // let agent_check = AgentCheck::new(
        //     "check_id",
        //     init_config_str,
        //     instance_config_str,
        //     aggregator_ptr
        // )?;

        // agent_check.check()

        Ok(())
    }
}