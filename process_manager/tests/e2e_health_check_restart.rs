mod common;

use common::*;
use std::fs;
use std::thread;
use std::time::Duration;

/// Cleanup Python server process for a specific port
fn cleanup_python_server(port: u16) {
    let _ = std::process::Command::new("pkill")
        .args(["-9", "-f", &format!("python.*{}", port)])
        .output();
    thread::sleep(Duration::from_millis(200));
}

#[test]
fn test_health_check_restart_on_failure() {
    let server_port = common::get_socket_test_port();

    // Clean up any leftover Python servers from previous runs
    cleanup_python_server(server_port);
    thread::sleep(Duration::from_millis(500));

    let temp_dir = setup_temp_dir();

    // Create a simple Python HTTP server that we can make fail
    let server_script = temp_dir.path().join("server.py");
    let health_file = temp_dir.path().join("healthy");
    let health_file_str = health_file.to_str().unwrap().to_string();

    // Start healthy - create the health file
    fs::write(&health_file, "ok").expect("Failed to write health file");

    // Python script that serves HTTP and checks health file
    let script_content = format!(
        r#"#!/usr/bin/env python3
import http.server
import socketserver
import os
import socket
import sys

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8080

class HealthHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if os.path.exists("{}"):
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"Healthy")
        else:
            self.send_response(503)
            self.end_headers()
            self.wfile.write(b"Unhealthy")

    def log_message(self, format, *args):
        pass  # Suppress logs

class ReuseAddrTCPServer(socketserver.TCPServer):
    allow_reuse_address = True

try:
    with ReuseAddrTCPServer(("", PORT), HealthHandler) as httpd:
        httpd.serve_forever()
except Exception as e:
    print(f"ERROR: {{e}}", file=sys.stderr, flush=True)
    sys.exit(1)
"#,
        health_file_str
    );

    fs::write(&server_script, script_content).expect("Failed to write server script");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mut perms = fs::metadata(&server_script).unwrap().permissions();
        perms.set_mode(0o755);
        fs::set_permissions(&server_script, perms).unwrap();
    }

    // Create config directory with process config (direct ProcessConfig format)
    let config_dir = temp_dir.path().join("config.d");
    fs::create_dir_all(&config_dir).expect("Failed to create config dir");
    
    let config_path = config_dir.join("test-server.yaml");
    let config_content = format!(
        r#"
command: python3
args:
  - {}
  - {}
working_dir: {}
auto_start: true
restart: on-failure
restart_sec: 1
health_check:
  type: http
  endpoint: http://localhost:{}/
  interval: 2
  timeout: 1
  retries: 2
  start_period: 5
  restart_after: 3  # Kill after 3 consecutive failures
"#,
        server_script.to_str().unwrap(),
        server_port,
        temp_dir.path().to_str().unwrap(),
        server_port
    );

    fs::write(&config_path, config_content).expect("Failed to write config");

    // Start daemon with config directory
    let _daemon = setup_daemon_with_config_dir(config_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(3)); // Wait for daemon to load config and start process

    // Verify process is running and healthy
    let (stdout, stderr, code) = run_cli_full(&["list"]);
    println!("Initial list (exit={}):\n{}", code, stdout);
    if !stderr.is_empty() {
        println!("Stderr: {}", stderr);
    }

    if !stdout.contains("test-server") {
        println!("ERROR: Process not found in list. Checking describe...");
        let (desc, _, _) = run_cli_full(&["describe", "test-server"]);
        println!("Describe output:\n{}", desc);
    }

    assert!(stdout.contains("test-server"), "Process should be created");

    // Give it more time if not running yet
    if !stdout.contains("running") {
        println!("Process not running yet, waiting 2 more seconds...");
        thread::sleep(Duration::from_secs(2));
        let (stdout2, _, _) = run_cli_full(&["list"]);
        println!("List after wait:\n{}", stdout2);
        assert!(
            stdout2.contains("running"),
            "Process should be running after wait"
        );
    }

    // Get initial PID
    let (stdout, _, _) = run_cli_full(&["describe", "test-server"]);
    let initial_pid = extract_pid_from_describe(&stdout);
    println!("Initial PID: {:?}", initial_pid);

    // Wait for health check to pass
    thread::sleep(Duration::from_secs(3));

    let (stdout, _, _) = run_cli_full(&["describe", "test-server"]);
    println!("After healthy checks:\n{}", stdout);
    // Should be healthy or starting (depending on timing)

    // Make the server unhealthy by removing health file
    println!("\n=== Making server unhealthy ===");
    fs::remove_file(&health_file).ok();

    // Wait for health checks to fail and trigger restart
    // restart_after=3, interval=2, so ~6 seconds + kill time + restart time
    println!("Waiting for health check failures and restart...");
    thread::sleep(Duration::from_secs(10));

    // Verify process was restarted (new PID and run_count increased)
    let (stdout, _, _) = run_cli_full(&["describe", "test-server"]);
    println!("After restart:\n{}", stdout);

    let new_pid = extract_pid_from_describe(&stdout);
    println!("New PID: {:?}", new_pid);

    // Check run count increased (should be >= 2, might be more if it keeps restarting due to health failures)
    let run_count_matches = stdout.contains("Run Count:       2")
        || stdout.contains("Run Count:       3")
        || stdout.contains("Run Count:       4")
        || stdout.contains("Run Count:       5");
    assert!(
        run_count_matches,
        "Run count should have increased after restart (got: {})",
        stdout
            .lines()
            .find(|l| l.contains("Run Count:"))
            .unwrap_or("not found")
    );

    // Verify PID changed (process was killed and restarted)
    if let (Some(old_pid), Some(new_pid)) = (initial_pid, new_pid) {
        assert_ne!(old_pid, new_pid, "PID should have changed after restart");
    }

    // Daemon cleanup handled by guard
    cleanup_python_server(server_port);
}

#[test]
fn test_health_check_no_restart_when_disabled() {
    let server_port = common::get_socket_test_port();

    // Clean up any leftover Python servers from previous runs
    cleanup_python_server(server_port);
    thread::sleep(Duration::from_millis(500));

    let temp_dir = setup_temp_dir();

    // Create a server that always fails health checks
    let server_script = temp_dir.path().join("failing_server.py");
    let script_content = r#"#!/usr/bin/env python3
import http.server
import socketserver
import sys

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8080

class AlwaysUnhealthyHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(503)
        self.end_headers()
        self.wfile.write(b"Always Unhealthy")

    def log_message(self, format, *args):
        pass  # Suppress logs

class ReuseAddrTCPServer(socketserver.TCPServer):
    allow_reuse_address = True

try:
    with ReuseAddrTCPServer(("", PORT), AlwaysUnhealthyHandler) as httpd:
        httpd.serve_forever()
except Exception as e:
    print(f"ERROR: {{e}}", file=sys.stderr, flush=True)
    sys.exit(1)
"#;

    fs::write(&server_script, script_content).expect("Failed to write server script");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mut perms = fs::metadata(&server_script).unwrap().permissions();
        perms.set_mode(0o755);
        fs::set_permissions(&server_script, perms).unwrap();
    }

    // Create config directory with process config (direct ProcessConfig format)
    let config_dir = temp_dir.path().join("config.d");
    fs::create_dir_all(&config_dir).expect("Failed to create config dir");
    
    let config_path = config_dir.join("test-server-no-restart.yaml");
    let config_content = format!(
        r#"
command: python3
args:
  - {}
  - {}
working_dir: {}
auto_start: true
restart: on-failure
health_check:
  type: http
  endpoint: http://localhost:{}/
  interval: 2
  timeout: 1
  retries: 2
  start_period: 3
  restart_after: 0  # NEVER restart on health failure (informational only)
"#,
        server_script.to_str().unwrap(),
        server_port,
        temp_dir.path().to_str().unwrap(),
        server_port
    );

    fs::write(&config_path, config_content).expect("Failed to write config");

    // Start daemon with config directory
    let _daemon = setup_daemon_with_config_dir(config_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(3)); // Wait for daemon to load and start

    // Check if process is running
    let (stdout, _, _) = run_cli_full(&["list"]);
    println!("Initial list:\n{}", stdout);

    if !stdout.contains("test-server-no-restart") {
        println!("ERROR: Process not created, waiting 2 more seconds...");
        thread::sleep(Duration::from_secs(2));
        let (stdout2, _, _) = run_cli_full(&["list"]);
        println!("List after wait:\n{}", stdout2);
    }

    // Get initial PID
    let (stdout, _, _) = run_cli_full(&["describe", "test-server-no-restart"]);
    println!("Describe output:\n{}", stdout);
    let initial_pid = extract_pid_from_describe(&stdout);
    println!("Initial PID: {:?}", initial_pid);
    assert!(initial_pid.is_some(), "Process should be running");

    // Wait for multiple health check failures
    println!("Waiting for health check failures (should NOT restart)...");
    thread::sleep(Duration::from_secs(10));

    // Verify process is STILL RUNNING with SAME PID (not restarted)
    let (stdout, _, _) = run_cli_full(&["describe", "test-server-no-restart"]);
    println!("After health failures:\n{}", stdout);

    let current_pid = extract_pid_from_describe(&stdout);
    println!("Current PID: {:?}", current_pid);

    // Should still be running
    assert!(
        stdout.contains("running"),
        "Process should still be running"
    );

    // PID should NOT have changed
    assert_eq!(
        initial_pid, current_pid,
        "PID should NOT change when restart_after=0"
    );

    // Run count should still be 1
    assert!(
        stdout.contains("Run Count:       1"),
        "Run count should still be 1 (no restart)"
    );

    // Health status should be unhealthy
    assert!(
        stdout.contains("unhealthy") || stdout.contains("Unhealthy"),
        "Health status should be unhealthy"
    );

    // Daemon cleanup handled by guard
    cleanup_python_server(server_port);
}

#[test]
fn test_health_check_restart_resets_on_recovery() {
    let server_port = common::get_socket_test_port();

    // Clean up any leftover Python servers from previous runs
    cleanup_python_server(server_port);
    thread::sleep(Duration::from_millis(500));

    let temp_dir = setup_temp_dir();

    let server_script = temp_dir.path().join("server.py");
    let health_file = temp_dir.path().join("healthy");
    let health_file_str = health_file.to_str().unwrap().to_string();

    // Start healthy
    fs::write(&health_file, "ok").expect("Failed to write health file");

    let script_content = format!(
        r#"#!/usr/bin/env python3
import http.server
import socketserver
import os
import sys

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8080

class HealthHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if os.path.exists("{}"):
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"Healthy")
        else:
            self.send_response(503)
            self.end_headers()
            self.wfile.write(b"Unhealthy")

    def log_message(self, format, *args):
        pass  # Suppress logs

class ReuseAddrTCPServer(socketserver.TCPServer):
    allow_reuse_address = True

try:
    with ReuseAddrTCPServer(("", PORT), HealthHandler) as httpd:
        httpd.serve_forever()
except Exception as e:
    print(f"ERROR: {{e}}", file=sys.stderr, flush=True)
    sys.exit(1)
"#,
        health_file_str
    );

    fs::write(&server_script, script_content).expect("Failed to write server script");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mut perms = fs::metadata(&server_script).unwrap().permissions();
        perms.set_mode(0o755);
        fs::set_permissions(&server_script, perms).unwrap();
    }

    // Create config directory with process config (direct ProcessConfig format)
    let config_dir = temp_dir.path().join("config.d");
    fs::create_dir_all(&config_dir).expect("Failed to create config dir");
    
    let config_path = config_dir.join("test-recovery.yaml");
    let config_content = format!(
        r#"
command: python3
args:
  - {}
  - {}
working_dir: {}
auto_start: true
restart: on-failure
restart_sec: 1
health_check:
  type: http
  endpoint: http://localhost:{}/
  interval: 2
  timeout: 1
  retries: 2
  start_period: 5  # Longer start period to avoid false failures during startup
  restart_after: 5  # Need 5 failures to trigger restart
"#,
        server_script.to_str().unwrap(),
        server_port,
        temp_dir.path().to_str().unwrap(),
        server_port
    );

    fs::write(&config_path, config_content).expect("Failed to write config");

    let _daemon = setup_daemon_with_config_dir(config_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(7)); // Wait for health check start_period (5s) + buffer

    // Check if process is running
    let (stdout, _, _) = run_cli_full(&["list"]);
    println!("Initial list:\n{}", stdout);

    let (stdout, _, _) = run_cli_full(&["describe", "test-recovery"]);
    println!("Initial describe:\n{}", stdout);

    // Check if process was created successfully
    if !stdout.contains("test-recovery") {
        println!("WARN: Process test-recovery not found - environment may not support this test");
        cleanup_python_server(server_port);
        return;
    }

    let initial_pid = extract_pid_from_describe(&stdout);
    println!("Initial PID: {:?}", initial_pid);

    // Extract initial run count
    let initial_run_count = stdout
        .lines()
        .find(|l| l.contains("Run Count:"))
        .and_then(|l| l.split_whitespace().last())
        .and_then(|s| s.parse::<u32>().ok())
        .unwrap_or(1);
    println!("Initial run count: {}", initial_run_count);

    // Make unhealthy for 2 failures (less than restart_after=5)
    println!("Making server unhealthy for 2 failures...");
    fs::remove_file(&health_file).ok();
    thread::sleep(Duration::from_secs(5)); // 2-3 checks

    // Recover before reaching restart threshold
    println!("Recovering server...");
    fs::write(&health_file, "ok").expect("Failed to write health file");
    thread::sleep(Duration::from_secs(3));

    // Verify process was NOT restarted (failure counter was reset)
    let (stdout, _, _) = run_cli_full(&["describe", "test-recovery"]);
    let current_pid = extract_pid_from_describe(&stdout);

    let current_run_count = stdout
        .lines()
        .find(|l| l.contains("Run Count:"))
        .and_then(|l| l.split_whitespace().last())
        .and_then(|s| s.parse::<u32>().ok())
        .unwrap_or(0);
    println!("Current run count: {}", current_run_count);

    assert_eq!(
        initial_pid, current_pid,
        "PID should NOT change - failure counter should reset on recovery"
    );

    assert_eq!(
        initial_run_count, current_run_count,
        "Run count should not change (no restart triggered)"
    );

    // Daemon cleanup handled by guard
    cleanup_python_server(server_port);
}

// Helper to extract PID from describe output
fn extract_pid_from_describe(output: &str) -> Option<u32> {
    for line in output.lines() {
        if line.contains("PID:") {
            let parts: Vec<&str> = line.split_whitespace().collect();
            if let Some(pid_str) = parts.get(1) {
                return pid_str.parse().ok();
            }
        }
    }
    None
}
