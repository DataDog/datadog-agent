//! E2E tests for configuration loading precedence and directory support
//!
//! Tests configuration loading scenarios:
//! 1. DD_PM_CONFIG_DIR (explicit directory)
//! 2. Directory with multiple files (load order)
//!
//! NOTE: Single-file configuration is not supported. Only directory-based
//! configuration with one YAML file per process is allowed.

use pm_e2e_tests::{get_daemon_binary, run_cli_full, setup_daemon_with_config_dir};
use std::fs;
use std::process::{Command, Stdio};
use std::thread;
use std::time::Duration;
use tempfile::TempDir;

#[test]
fn test_config_dir_explicit() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let processes_dir = temp_dir.path().join("processes.d");
    fs::create_dir(&processes_dir).expect("Failed to create processes.d");

    // Create a test config file in the directory (direct ProcessConfig format)
    // Process name is derived from filename: nginx.yaml -> process name "nginx"
    let config_path = processes_dir.join("nginx.yaml");
    fs::write(
        &config_path,
        r#"
# Direct ProcessConfig format (no 'processes:' wrapper)
command: /bin/echo
args: ["nginx"]
auto_start: false
"#,
    )
    .expect("Failed to write config");

    let _daemon = setup_daemon_with_config_dir(processes_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(1));

    // Verify process was loaded (name from filename)
    let (stdout, _stderr, code) = run_cli_full(&["list"]);
    assert_eq!(code, 0);
    assert!(stdout.contains("nginx"), "nginx should be loaded");

    // Daemon cleanup handled by guard
}

#[test]
fn test_config_dir_rejects_file() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let config_file = temp_dir.path().join("test.yaml");

    // Create a single config file (not a directory)
    fs::write(&config_file, "command: /bin/echo\nauto_start: false").expect("Failed to write config");

    // Try to start daemon with a file path instead of directory (should fail)
    let result = Command::new(get_daemon_binary())
        .env("DD_PM_CONFIG_DIR", config_file.to_str().unwrap())
        .env("DD_PM_TRANSPORT_MODE", "tcp")
        .env("DD_PM_GRPC_PORT", "59999")
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn();

    assert!(result.is_ok(), "Daemon should start (to check error)");
    let mut child = result.unwrap();

    // Wait briefly and check if it exited with error
    thread::sleep(Duration::from_millis(500));
    let status = child.wait().expect("Failed to wait for daemon");

    assert!(
        !status.success(),
        "Daemon should exit with error when DD_PM_CONFIG_DIR is a file"
    );

    // Clean up
    let _ = Command::new("pkill")
        .arg("-f")
        .args(["--port", "59999"])
        .output();
}

#[test]
fn test_directory_load_order() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let processes_dir = temp_dir.path().join("processes.d");
    fs::create_dir(&processes_dir).expect("Failed to create processes.d");

    // Create multiple files (alphabetical order should be preserved)
    // Process names are derived from filenames: 01-first.yaml -> "01-first"
    fs::write(
        processes_dir.join("01-first.yaml"),
        r#"
# Direct ProcessConfig format (no 'processes:' wrapper)
command: /bin/echo
args: ["first"]
auto_start: false
"#,
    )
    .expect("Failed to write first config");

    fs::write(
        processes_dir.join("02-second.yaml"),
        r#"
# Direct ProcessConfig format
command: /bin/echo
args: ["second"]
auto_start: false
"#,
    )
    .expect("Failed to write second config");

    let _daemon = setup_daemon_with_config_dir(processes_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(1));

    // Verify both processes were loaded (names from filenames)
    let (stdout, _stderr, code) = run_cli_full(&["list"]);
    assert_eq!(code, 0);
    assert!(stdout.contains("01-first"), "01-first should be loaded");
    assert!(stdout.contains("02-second"), "02-second should be loaded");

    // Daemon cleanup handled by guard
}

#[test]
fn test_directory_yml_extension() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let processes_dir = temp_dir.path().join("processes.d");
    fs::create_dir(&processes_dir).expect("Failed to create processes.d");

    // Create a file with .yml extension (should also work)
    // Process name derived from filename: test.yml -> "test"
    fs::write(
        processes_dir.join("yml-test.yml"),
        r#"
# Direct ProcessConfig format
command: /bin/echo
args: ["yml-test"]
auto_start: false
"#,
    )
    .expect("Failed to write yml config");

    let _daemon = setup_daemon_with_config_dir(processes_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(1));

    // Verify process was loaded (name from filename)
    let (stdout, _stderr, code) = run_cli_full(&["list"]);
    assert_eq!(code, 0);
    assert!(
        stdout.contains("yml-test"),
        ".yml extension should be supported"
    );

    // Daemon cleanup handled by guard
}

#[test]
#[ignore = "Daemon doesn't support CLI arguments, only environment variables"]
fn test_help_flag() {
    // Test that --help works
    let output = Command::new(get_daemon_binary())
        .arg("--help")
        .output()
        .expect("Failed to run daemon --help");

    assert!(output.status.success(), "Help flag should succeed");
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Usage") || stdout.contains("usage") || stdout.contains("--help"),
        "Help output should contain usage information"
    );
}
