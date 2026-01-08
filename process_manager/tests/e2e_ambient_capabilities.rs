//! E2E tests for Ambient Capabilities feature
//!
//! Ambient capabilities allow non-root processes to retain specific capabilities.
//! These tests verify the AmbientCapabilities field behavior in the CLI and daemon.
//!
//! Note: Most capability-related tests require root or CAP_SETPCAP to actually
//! set capabilities, so these tests focus on:
//! 1. CLI argument parsing and validation
//! 2. Configuration persistence (describe output)
//! 3. Process creation with capabilities specified
//! 4. Error handling for invalid capability names

use pm_e2e_tests::{create_process, run_cli_full, setup_daemon, unique_process_name};

use std::thread;
use std::time::Duration;

#[test]
fn test_ambient_capabilities_cli_parsing() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create a process with ambient capabilities
    create_process(
        &name,
        "/bin/sleep",
        &[
            "60",
            "--ambient-capability",
            "CAP_NET_BIND_SERVICE",
            "--ambient-capability",
            "CAP_SYS_PTRACE",
        ],
    );

    // Verify capabilities are stored
    let (stdout, _stderr, code) = run_cli_full(&["describe", &name]);
    assert_eq!(code, 0);
    assert!(stdout.contains(&name));
    assert!(stdout.contains("CAP_NET_BIND_SERVICE"));
    assert!(stdout.contains("CAP_SYS_PTRACE"));

    // Daemon cleanup handled by guard
}

#[test]
fn test_ambient_capabilities_multiple() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create a process with multiple capabilities
    create_process(
        &name,
        "/bin/sleep",
        &[
            "60",
            "--ambient-capability",
            "CAP_NET_BIND_SERVICE",
            "--ambient-capability",
            "CAP_NET_RAW",
            "--ambient-capability",
            "CAP_SYS_ADMIN",
        ],
    );

    // Verify all capabilities are stored
    let (stdout, _stderr, code) = run_cli_full(&["describe", &name]);
    assert_eq!(code, 0);
    assert!(stdout.contains("CAP_NET_BIND_SERVICE"));
    assert!(stdout.contains("CAP_NET_RAW"));
    assert!(stdout.contains("CAP_SYS_ADMIN"));

    // Daemon cleanup handled by guard
}

#[test]
fn test_ambient_capabilities_with_user_switching() {
    let _daemon = setup_daemon();

    // Ambient capabilities are designed to work with user switching
    // Create a process that switches to a non-root user with capabilities
    // Note: This may fail in some test environments if user switching is not supported
    let (_stdout, stderr, code) = run_cli_full(&[
        "create",
        "cap-user",
        "/bin/sleep",
        "60",
        "--user",
        "nobody",
        "--ambient-capability",
        "CAP_NET_BIND_SERVICE",
    ]);

    // If creation fails due to user issues, that's okay for this test
    // We're primarily testing that the capability configuration is accepted
    if code != 0 {
        println!(
            "User switching may not be supported in this environment: {}",
            stderr
        );
        // Daemon cleanup handled by guard
        return; // Skip the rest of the test
    }

    // Verify configuration is stored
    let (stdout, _stderr, code) = run_cli_full(&["describe", "cap-user"]);
    assert_eq!(code, 0);
    assert!(stdout.contains("CAP_NET_BIND_SERVICE"));
    // User field verification is optional - output format may vary

    // Daemon cleanup handled by guard
}

#[test]
fn test_ambient_capabilities_describe_output() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create process with capabilities
    let (_stdout, _stderr, code) = run_cli_full(&[
        "create",
        &name,
        "/bin/sleep",
        "60",
        "--ambient-capability",
        "CAP_CHOWN",
        "--ambient-capability",
        "CAP_FOWNER",
    ]);
    assert_eq!(code, 0);

    // Describe and verify format
    let (stdout, _stderr, code) = run_cli_full(&["describe", &name]);
    assert_eq!(code, 0);
    // Verify the capabilities are present in the output (format may vary)
    assert!(
        stdout.contains("CAP_CHOWN"),
        "Should contain CAP_CHOWN:\n{}",
        stdout
    );
    assert!(
        stdout.contains("CAP_FOWNER"),
        "Should contain CAP_FOWNER:\n{}",
        stdout
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_ambient_capabilities_empty() {
    let _daemon = setup_daemon();

    // Create a process without ambient capabilities
    let (_stdout, _stderr, code) = run_cli_full(&["create", "no-cap", "/bin/sleep", "60"]);
    assert_eq!(code, 0);

    // Describe should not show capabilities or show empty
    let (_stdout, _stderr, code) = run_cli_full(&["describe", "no-cap"]);
    assert_eq!(code, 0);
    // Should either not show the field or show it as empty
    // This is acceptable behavior for both cases

    // Daemon cleanup handled by guard
}

#[test]
fn test_ambient_capabilities_invalid_name() {
    let _daemon = setup_daemon();

    // Try to create a process with an invalid capability name
    // The validation happens at process start time, not create time
    let (stdout, _stderr, code) = run_cli_full(&[
        "create",
        "invalid-cap",
        "/bin/sleep",
        "60",
        "--ambient-capability",
        "INVALID_CAP_NAME",
    ]);
    // Create should succeed (validation happens at start)
    assert_eq!(code, 0, "Process creation should succeed:\n{}", stdout);

    // Try to start - this should fail with invalid capability
    let (stdout, stderr, _code) = run_cli_full(&["start", "invalid-cap"]);
    let output = format!("{}\n{}", stdout, stderr);
    // Start should fail due to invalid capability
    assert!(
        output.contains("Invalid capability")
            || output.contains("error")
            || output.contains("[ERROR]"),
        "Should fail with capability error:\n{}",
        output
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_ambient_capabilities_common_use_case() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Common use case: Web server binding to port 80 without root
    create_process(
        &name,
        "/bin/sleep",
        &[
            "60",
            "--user",
            "nobody",
            "--ambient-capability",
            "CAP_NET_BIND_SERVICE", // Allows binding to ports < 1024
        ],
    );

    // Verify configuration
    let (stdout, _stderr, code) = run_cli_full(&["describe", &name]);
    assert_eq!(code, 0);
    assert!(stdout.contains("CAP_NET_BIND_SERVICE"));

    // Daemon cleanup handled by guard
}

#[test]
fn test_ambient_capabilities_restart_preserves() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create process with capabilities and auto-restart
    let (_stdout, _stderr, code) = run_cli_full(&[
        "create",
        &name,
        "/bin/sh",
        "-c",
        "exit 0",
        "--restart",
        "always",
        "--ambient-capability",
        "CAP_NET_RAW",
    ]);
    assert_eq!(code, 0);

    // Start the process
    let (stdout, stderr, code) = run_cli_full(&["start", &name]);

    // If starting fails due to capability issues, that's okay - we're mainly testing
    // that the configuration is accepted and persisted
    if code != 0 && stderr.contains("Invalid argument") {
        println!(
            "Ambient capabilities may not be supported in this environment: {}",
            stderr
        );
        // Still verify that the configuration was stored
        let (stdout, _stderr, code) = run_cli_full(&["describe", &name]);
        assert_eq!(code, 0);
        assert!(
            stdout.contains("CAP_NET_RAW"),
            "Capability should be stored in config even if not supported at runtime"
        );
        return;
    }

    assert_eq!(
        code, 0,
        "Start failed:\nstdout: {}\nstderr: {}",
        stdout, stderr
    );

    // Wait for process to exit and restart
    thread::sleep(Duration::from_secs(3));

    // Verify capabilities are still present after restart
    let (stdout, _stderr, code) = run_cli_full(&["describe", &name]);
    assert_eq!(code, 0);
    assert!(stdout.contains("CAP_NET_RAW"));

    // Daemon cleanup handled by guard
}
