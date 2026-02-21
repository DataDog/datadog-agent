use core::*;
use std::error::Error;

use crate::url::Url;
use crate::certs::inspect_cert;

use std::time::Instant;
use std::sync::Arc;
use std::net::{TcpStream, ToSocketAddrs};
use std::io::{ErrorKind, Read, Write};
use std::time::Duration;

use rustls::{KeyLogFile, ClientConnection, RootCertStore, Stream};
use rustls::pki_types::{CertificateDer, ServerName};
use webpki_roots::TLS_SERVER_ROOTS;

/// Check implementation
pub fn check(check: &AgentCheck) -> Result<(), Box<dyn Error>> {
    // ssl certificates can be collected during the execution of the check
    // if not, a service check will be sent
    let mut peer_cert: Option<CertificateDer> = None;

    // parse the url given by the configuration
    let url_str: String = check.instance.get("url")?;
    let url = Url::from(&url_str);
    
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
        .next().unwrap(); // TODO: refactor the unwrap to catch the panic case

    // TODO: refactor the match statement nesting
    match TcpStream::connect_timeout(&addr, Duration::from_secs(5)) {
        Ok(mut sock) => {                
            // Create the TLS stream
            let mut stream = Stream::new(&mut conn, &mut sock);

            // http request header
            // TODO: use another crate to avoid writing the HTTP request like `ureq` or `reqwest`
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
                        check.gauge("network.http.response_time", elapsed_time.as_secs_f64(), &tags, "", false)?;
                    }

                    // read response from the TLS stream
                    let mut response_raw = Vec::new();

                    match stream.read_to_end(&mut response_raw) {
                        Ok(_) => {
                            let response = String::from_utf8(response_raw)?;

                            // status code
                            let first_line = response.lines().nth(0).unwrap(); // TODO: refactor the unwrap to catch the panic case
                            let status_code: u32 = first_line
                                .split_whitespace()
                                .nth(1).unwrap() // TODO: refactor the unwrap to catch the panic case
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

        check.gauge("network.http.can_connect", can_connect, &tags, "", false)?;
        check.gauge("network.http.cant_connect", cant_connect, &tags, "", true)?;
    }

    // get certificate expiration info
    // TODO: do it only if the URL scheme is HTTPS
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
        check.gauge("http.ssl.days_left", days_left as f64, &tags, "", false)?;
    }

    if let Some(seconds_left) = seconds_left {
        check.gauge("http.ssl.seconds_left", seconds_left as f64, &tags, "", true)?;
    }

    // add ssl service check for certificate expiration
    service_checks.push((
        "http.ssl_cert".to_string(),
        status,
        msg,
    ));

    // submit every service check collected throughout the check
    for (sc_name, status, message) in service_checks {
        check.service_check(&sc_name, status, &service_checks_tags, "", &message)?;
    }

    Ok(())
}
