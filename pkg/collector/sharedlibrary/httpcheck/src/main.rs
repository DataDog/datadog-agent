// this file is only used as a sketch for the future implementation in lib.rs

mod utils;
use utils::base::{AgentCheck, CheckID, ServiceCheckStatus};

use std::any::Any;
use std::error::Error;
use std::io::{Read, Write, ErrorKind};
use std::net::{TcpStream, ToSocketAddrs};
use std::sync::Arc;
use std::time::{Instant, Duration};

use rustls::{KeyLogFile, ClientConnection, RootCertStore, Stream};
use webpki_roots::TLS_SERVER_ROOTS;

fn main() {
    // hardcoded variables (should be passed as parameters inside a struct)
    let url = "datadog.com";
    let response_time = true;
    let ssl_expire = true;
    let uri_scheme = "https";

    let mut tags = Vec::<String>::new();

    // list service checks and their custom tags
    let mut service_checks = Vec::<(String, ServiceCheckStatus, String)>::new();
    let service_checks_tags = Vec::<String>::new(); // need to be set equal to the tags list at the beginning

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
            // Set timeouts
            // TODO: handle timeout errors later in the code
            sock.set_read_timeout(Some(Duration::from_secs(5))).unwrap();
            sock.set_write_timeout(Some(Duration::from_secs(5))).unwrap();
            
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

            println!("Elapsed time: {}ms", elapsed_time.as_millis());

            match response_result {
                Ok(()) => {
                    // Read the response from the TLS stream
                    let mut response = Vec::new();

                    match tls.read_to_end(&mut response) {
                        Ok(_) => {/* The server response has been successfuly read */},
                        Err(e) => {
                            println!("Error kind: {}", e.kind());
                            if e.kind() == ErrorKind::TimedOut {
                                println!("Timeout while reading server response: {e}")
                            } else {
                                println!("Error while reading server response: {e}")
                            }
                        },
                    }
                    let response = String::from_utf8(response[..].to_vec()).unwrap();

                    // Handling of status code
                    let first_line = response.lines().nth(0).unwrap();
                    let status_code: u32 = first_line
                        .split_whitespace()
                        .nth(1).unwrap()
                        .parse().unwrap();


                    if status_code >= 400 {
                        println!("HTTP Error: {}", status_code);
                    }

                    // TODO: handle SSL certificates here
                    match tls.conn.peer_certificates() {
                        Some(certs) => {
                            for _cert in certs {
                                //println!("Certificate: {:?}", _cert);
                            }
                        }
                        None => {
                            println!("No peer certificates found.");
                        }
                    }
                },
                Err(e) => {
                    println!("Error kind: {}", e.kind());
                    if e.kind() == ErrorKind::TimedOut {
                        println!("Timeout while sending request: {e}")
                    } else {
                        println!("Error while sending request: {e}");
                    }
                },
            }
        },
        Err(e) => {
            if e.kind() == ErrorKind::TimedOut {
                println!("Timeout while sending request: {e}")
            } else {
                // connection error
                println!("Error while connecting to the server: {e}");
            }
        },
    }
}
