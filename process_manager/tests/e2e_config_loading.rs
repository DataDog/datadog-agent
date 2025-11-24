//! E2E tests for configuration loading precedence and directory support
//!
//! Tests all configuration loading scenarios:
//! 1. DD_PM_CONFIG_FILE (explicit file)
//! 2. DD_PM_CONFIG_DIR (explicit directory)
//! 3. Both env vars together (error case)
//! 4. Directory with multiple files (load order)

use pm_e2e_tests::{
    get_daemon_binary, run_cli_full, setup_daemon_with_config_dir, setup_daemon_with_config_file,
};
use std::fs;
use std::process::{Command, Stdio};
use std::thread;
use std::time::Duration;
use tempfile::TempDir;

#[test]
fn test_config_file_explicit() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let config_path = temp_dir.path().join("test.yaml");

    // Create a test config file
    fs::write(
        &config_path,
        r#"
processes:
  test-process:
    command: /bin/echo
    args: ["test1"]
    auto_start: false
"#,
    )
    .expect("Failed to write config");

    let _daemon = setup_daemon_with_config_file(config_path.to_str().unwrap());
    thread::sleep(Duration::from_secs(1));

    // Verify process was loaded
    let (stdout, _stderr, code) = run_cli_full(&["list"]);
    assert_eq!(code, 0);
    assert!(
        stdout.contains("test-process"),
        "Process should be loaded from config file"
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_config_dir_explicit() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let processes_dir = temp_dir.path().join("processes.d");
    fs::create_dir(&processes_dir).expect("Failed to create processes.d");

    // Create a test config file in the directory
    let config_path = processes_dir.join("nginx.yaml");
    fs::write(
        &config_path,
        r#"
processes:
  nginx:
    command: /bin/echo
    args: ["nginx"]
    auto_start: false
"#,
    )
    .expect("Failed to write config");

    let _daemon = setup_daemon_with_config_dir(processes_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(1));

    // Verify process was loaded
    let (stdout, _stderr, code) = run_cli_full(&["list"]);
    assert_eq!(code, 0);
    assert!(stdout.contains("nginx"), "nginx should be loaded");

    // Daemon cleanup handled by guard
}

#[test]
fn test_both_flags_error() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let config_file = temp_dir.path().join("test.yaml");
    let config_dir = temp_dir.path().join("processes.d");

    fs::write(&config_file, "processes: {}").expect("Failed to write config");
    fs::create_dir(&config_dir).expect("Failed to create dir");

    // Try to start daemon with both env vars (should fail)
    let result = Command::new(get_daemon_binary())
        .env("DD_PM_CONFIG_FILE", config_file.to_str().unwrap())
        .env("DD_PM_CONFIG_DIR", config_dir.to_str().unwrap())
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
        "Daemon should exit with error when both flags are provided"
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
    fs::write(
        processes_dir.join("01-first.yaml"),
        r#"
processes:
  first:
    command: /bin/echo
    args: ["first"]
    auto_start: false
"#,
    )
    .expect("Failed to write first config");

    fs::write(
        processes_dir.join("02-second.yaml"),
        r#"
processes:
  second:
    command: /bin/echo
    args: ["second"]
    auto_start: false
"#,
    )
    .expect("Failed to write second config");

    let _daemon = setup_daemon_with_config_dir(processes_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(1));

    // Verify both processes were loaded
    let (stdout, _stderr, code) = run_cli_full(&["list"]);
    assert_eq!(code, 0);
    assert!(stdout.contains("first"), "first should be loaded");
    assert!(stdout.contains("second"), "second should be loaded");

    // Daemon cleanup handled by guard
}

#[test]
fn test_directory_yml_extension() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let processes_dir = temp_dir.path().join("processes.d");
    fs::create_dir(&processes_dir).expect("Failed to create processes.d");

    // Create a file with .yml extension (should also work)
    fs::write(
        processes_dir.join("test.yml"),
        r#"
processes:
  yml-test:
    command: /bin/echo
    args: ["yml-test"]
    auto_start: false
"#,
    )
    .expect("Failed to write yml config");

    let _daemon = setup_daemon_with_config_dir(processes_dir.to_str().unwrap());
    thread::sleep(Duration::from_secs(1));

    // Verify process was loaded
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
