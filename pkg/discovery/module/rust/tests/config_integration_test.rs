// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#![allow(clippy::unwrap_used)]
#![allow(clippy::expect_used)]
#![allow(clippy::cast_possible_wrap)]

use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::process::Command;
use std::thread;
use std::time::Duration;
use tempfile::TempDir;

const SYSTEM_PROBE_LITE_BIN: &str = env!("CARGO_BIN_EXE_system-probe-lite");

fn spawn_spl(temp_dir: &TempDir) -> (std::process::Child, std::path::PathBuf) {
    let socket_path = temp_dir.path().join("sysprobe.sock");
    let log_path = temp_dir.path().join("system-probe.log");

    let child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .arg("run")
        .arg("--socket")
        .arg(&socket_path)
        .arg("--log-file")
        .arg(&log_path)
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    // Give it time to start and bind the socket
    thread::sleep(Duration::from_millis(500));

    (child, socket_path)
}

/// Send a GET request over a Unix socket and return (status_line, headers, body).
fn http_get(socket_path: &std::path::Path, path: &str) -> (String, String, String) {
    let mut stream = UnixStream::connect(socket_path).expect("Failed to connect to socket");
    stream
        .set_read_timeout(Some(Duration::from_secs(5)))
        .expect("Failed to set read timeout");

    let request = format!("GET {path} HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n");
    stream
        .write_all(request.as_bytes())
        .expect("Failed to write request");

    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .expect("Failed to read response");

    // Parse HTTP response: status line, headers, body
    let parts: Vec<&str> = response.splitn(2, "\r\n\r\n").collect();
    let (header_section, body) = if parts.len() == 2 {
        (parts[0], parts[1].to_string())
    } else {
        (parts[0], String::new())
    };

    let mut header_lines = header_section.split("\r\n");
    let status_line = header_lines.next().unwrap_or("").to_string();
    let headers: String = header_lines.collect::<Vec<&str>>().join("\r\n");

    (status_line, headers, body)
}

fn kill_child(child: &mut std::process::Child) {
    use nix::sys::signal::{self, Signal};
    use nix::unistd::Pid;
    signal::kill(Pid::from_raw(child.id() as i32), Signal::SIGTERM)
        .expect("Failed to send SIGTERM");
    child.wait().expect("Failed to wait on child");
}

#[test]
fn test_config_endpoint_returns_yaml() {
    let temp_dir = TempDir::new().unwrap();
    let (mut child, socket_path) = spawn_spl(&temp_dir);

    let (status_line, _headers, body) = http_get(&socket_path, "/config");

    assert!(
        status_line.contains("200"),
        "Expected 200 OK, got: {status_line}"
    );

    // Parse as YAML to verify structure
    let docs = yaml_rust2::YamlLoader::load_from_str(&body).expect("Failed to parse YAML");
    assert!(!docs.is_empty(), "Expected at least one YAML document");
    let doc = &docs[0];
    assert_eq!(
        doc["discovery"]["enabled"].as_bool(),
        Some(true),
        "discovery.enabled should be true"
    );
    assert_eq!(
        doc["discovery"]["use_system_probe_lite"].as_bool(),
        Some(true),
        "discovery.use_system_probe_lite should be true"
    );

    kill_child(&mut child);
}

#[test]
fn test_config_by_source_endpoint_returns_json() {
    let temp_dir = TempDir::new().unwrap();
    let (mut child, socket_path) = spawn_spl(&temp_dir);

    let (status_line, headers, body) = http_get(&socket_path, "/config/by-source");

    assert!(
        status_line.contains("200"),
        "Expected 200 OK, got: {status_line}"
    );

    // Verify Content-Type header
    let headers_lower = headers.to_lowercase();
    assert!(
        headers_lower.contains("content-type: application/json"),
        "Expected application/json content type, got headers: {headers}"
    );

    // Parse as JSON and verify structure
    let parsed: serde_json::Value =
        serde_json::from_str(&body).expect("Failed to parse JSON response");

    assert!(
        parsed.get("default").is_some(),
        "Expected 'default' key in response, got: {body}"
    );

    let default = &parsed["default"];
    assert_eq!(
        default["discovery"]["enabled"],
        serde_json::json!(true),
        "discovery.enabled should be true"
    );
    assert_eq!(
        default["discovery"]["use_system_probe_lite"],
        serde_json::json!(true),
        "discovery.use_system_probe_lite should be true"
    );

    kill_child(&mut child);
}
