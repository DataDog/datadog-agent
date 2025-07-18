// this file is only used as a sketch for the future implementation in lib.rs

use std::io::{Read, Write};
use std::net::TcpStream;
use std::sync::Arc;
use std::time::Duration;

use rustls::{ClientConnection, RootCertStore, Stream};

fn main() {
    let root_store = RootCertStore { roots: webpki_roots::TLS_SERVER_ROOTS.into()};

    let mut config = rustls::ClientConfig::builder()
        .with_root_certificates(root_store)
        .with_no_client_auth();

    // Allow using SSLKEYLOGFILE.
    config.key_log = Arc::new(rustls::KeyLogFile::new());

    let url = "github.com";

    let server_name = url.try_into().unwrap();

    // TODO: check error here to handle connection errors
    let mut conn = ClientConnection::new(Arc::new(config), server_name).expect("Failed to create TLS connection");

    // TODO: check error here to handle connection errors
    let mut sock = TcpStream::connect(format!("{}:443", url)).expect("Failed to connect to server");
    
    // Set timeouts
    // TODO: handle timeout errors later in the code
    sock.set_read_timeout(Some(Duration::from_secs(5))).unwrap();
    sock.set_write_timeout(Some(Duration::from_secs(5))).unwrap();
    
    // Create the TLS stream
    let mut tls = Stream::new(&mut conn, &mut sock);

    // Send the HTTP request with this TLS stream
    tls.write_all(
        format!("
GET / HTTP/1.1
Host: {}
Connection: close
Accept-Encoding: identity
\n
",
    url
    )
    .as_bytes(),
    )
    .expect("Failed to write request");

    // Read the response from the TLS stream
    let mut plaintext = Vec::new();
    match tls.read_to_end(&mut plaintext) {
        Ok(_) => println!("Response read successfully"),
        Err(e) => println!("Failed to read response: {}", e),
    }
    let response = String::from_utf8(plaintext).expect("Failed to convert response to String");
    
    // Handling of status code
    let first_line = response.lines().nth(0).expect("Failed to get first line of response");
    let status_code = first_line
        .split_whitespace().nth(1).expect("Failed to get status code at position 1")
        .trim().parse::<i32>().expect("Failed to parse status code");


    if status_code >= 400 {
        println!("HTTP Error: {}", status_code);
    }

    // TODO: habndle SSL certificates here
    match tls.conn.peer_certificates() {
        Some(certs) => {
            for _cert in certs {
                //println!("Certificate: {:?}", cert);
            }
        }
        None => {
            println!("No peer certificates found.");
        }
    }
}
