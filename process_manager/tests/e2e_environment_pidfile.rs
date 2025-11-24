//! End-to-end tests for EnvironmentFile= and PIDFile= features
#![allow(clippy::field_reassign_with_default)]

use pm_e2e_tests::{
    create_and_start_process, create_process, extract_process_id, run_cli, run_cli_full,
    setup_daemon, unique_process_name, unique_test_path,
};
use std::fs;
use std::path::Path;
use std::process::Command;
use std::thread;
use std::time::Duration;

#[test]
fn test_e2e_pidfile_creation_and_cleanup() {
    let _daemon = setup_daemon();

    let pidfile = unique_test_path("test_pidfile_e2e", ".pid");

    // Ensure PID file doesn't exist
    let _ = fs::remove_file(&pidfile);

    // Create and start process with PID file
    let name = unique_process_name();
    let process_id = create_and_start_process(&name, "sleep", &["30", "--pidfile", &pidfile]);
    thread::sleep(Duration::from_millis(500));

    // Verify PID file exists
    assert!(
        Path::new(&pidfile).exists(),
        "PID file should exist after start"
    );

    // Read PID from file
    let pid_content = fs::read_to_string(&pidfile).expect("Failed to read PID file");
    let pid: u32 = pid_content.trim().parse().expect("Invalid PID in file");
    assert!(pid > 0, "PID should be positive");

    // Verify process is actually running with that PID
    let ps_output = Command::new("ps")
        .args(["-p", &pid.to_string()])
        .output()
        .expect("Failed to run ps");
    assert!(
        ps_output.status.success(),
        "Process with PID from file should be running"
    );

    // Stop the process
    let output = run_cli(&["stop", &process_id]);
    assert!(
        output.contains("[OK]"),
        "Failed to stop process: {}",
        output
    );
    thread::sleep(Duration::from_secs(1));

    // Verify PID file is removed
    assert!(
        !Path::new(&pidfile).exists(),
        "PID file should be removed after stop"
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_environment_file_loading() {
    let _daemon = setup_daemon();

    let env_file = unique_test_path("test_env_e2e", ".env");

    // Create environment file
    fs::write(
        &env_file,
        "# Test environment file\nTEST_VAR_1=from_file\nTEST_VAR_2=value_two\nDD_API_KEY=file_key\n",
    )
    .expect("Failed to write env file");

    // Create process with environment file
    let name = unique_process_name();
    let process_id = create_process(
        &name,
        "/bin/sh",
        &[
            "-c",
            "echo TEST_VAR_1=$TEST_VAR_1; echo TEST_VAR_2=$TEST_VAR_2; echo DD_API_KEY=$DD_API_KEY",
            "--environment-file",
            &env_file,
            "--env",
            "DD_API_KEY=cli_override",
            "--stdout",
            "inherit",
        ],
    );

    // Check describe output to verify environment
    let output = run_cli(&["describe", &process_id]);

    // Verify variables from file are present
    assert!(
        output.contains("TEST_VAR_1=from_file"),
        "TEST_VAR_1 should be loaded from file"
    );
    assert!(
        output.contains("TEST_VAR_2=value_two"),
        "TEST_VAR_2 should be loaded from file"
    );

    // Verify CLI override works
    assert!(
        output.contains("DD_API_KEY=cli_override"),
        "DD_API_KEY should be overridden by CLI"
    );

    // Cleanup
    fs::remove_file(&env_file).ok();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_optional_environment_file_missing() {
    let _daemon = setup_daemon();

    let missing_file = "/tmp/does_not_exist_e2e.env";

    // Ensure file doesn't exist
    let _ = fs::remove_file(missing_file);

    // Create process with optional (prefix with '-') missing env file - should succeed
    let _process_id = create_process(
        "optional_env",
        "echo",
        &[
            "hello",
            "--environment-file",
            &format!("-{}", missing_file),
            "--env",
            "TEST=value",
        ],
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_required_environment_file_missing() {
    let _daemon = setup_daemon();

    let missing_file = "/tmp/required_missing_e2e.env";

    // Ensure file doesn't exist
    let _ = fs::remove_file(missing_file);

    // Create process with required (no '-' prefix) missing env file - should fail
    let (stdout, stderr, exit_code) = run_cli_full(&[
        "create",
        "required_env",
        "echo",
        "hello",
        "--environment-file",
        missing_file,
    ]);

    let output = format!("{}{}", stdout, stderr);
    // The create should fail (nonzero exit) OR contain an error message
    assert!(
        exit_code != 0
            || output.contains("error")
            || output.contains("Error")
            || output.contains("failed"),
        "Should fail or contain error message, got exit_code={}, output: {}",
        exit_code,
        output
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_environment_file_comments_and_empty_lines() {
    let _daemon = setup_daemon();

    let env_file = unique_test_path("test_env_comments_e2e", ".env");

    // Create environment file with comments and empty lines
    fs::write(
        &env_file,
        r#"# This is a comment
VAR1=value1

# Another comment
VAR2=value2

VAR3=value3
"#,
    )
    .expect("Failed to write env file");

    // Create process
    let name = unique_process_name();
    let process_id = create_process(
        &name,
        "/bin/sh",
        &[
            "-c",
            "echo VAR1=$VAR1; echo VAR2=$VAR2; echo VAR3=$VAR3",
            "--environment-file",
            &env_file,
            "--stdout",
            "inherit",
        ],
    );

    // Verify all variables loaded (comments and empty lines ignored)
    let output = run_cli(&["describe", &process_id]);
    assert!(output.contains("VAR1=value1"), "VAR1 should be loaded");
    assert!(output.contains("VAR2=value2"), "VAR2 should be loaded");
    assert!(output.contains("VAR3=value3"), "VAR3 should be loaded");

    // Cleanup
    fs::remove_file(&env_file).ok();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_pidfile_cleanup_on_crash() {
    let _daemon = setup_daemon();

    let pidfile = unique_test_path("test_pidfile_crash_e2e", ".pid");

    // Ensure PID file doesn't exist
    let _ = fs::remove_file(&pidfile);

    // Create and start process that will crash (exit code 1)
    let name = unique_process_name();
    let _process_id = create_and_start_process(
        &name,
        "/bin/sh",
        &["-c", "sleep 1; exit 1", "--pidfile", &pidfile],
    );

    // Verify PID file exists while running
    assert!(
        Path::new(&pidfile).exists(),
        "PID file should exist after start"
    );

    // Wait for crash
    thread::sleep(Duration::from_secs(3));

    // Verify PID file is cleaned up after crash
    assert!(
        !Path::new(&pidfile).exists(),
        "PID file should be removed after crash"
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_multiple_processes_with_pidfiles() {
    let _daemon = setup_daemon();

    let name1 = unique_process_name();
    let name2 = unique_process_name();

    let pidfile1 = unique_test_path("test_multi_pid1_e2e", ".pid");
    let pidfile2 = unique_test_path("test_multi_pid2_e2e", ".pid");

    // Ensure PID files don't exist
    let _ = fs::remove_file(&pidfile1);
    let _ = fs::remove_file(&pidfile2);

    // Create first process
    let output = run_cli(&["create", &name1, "sleep", "30", "--pidfile", &pidfile1]);
    assert!(
        output.contains("Process created"),
        "Failed to create {}: {}",
        name1,
        output
    );
    let process_id1 = extract_process_id(&output)
        .expect("Failed to extract process ID")
        .to_string();

    // Create second process
    let output = run_cli(&["create", &name2, "sleep", "30", "--pidfile", &pidfile2]);
    assert!(
        output.contains("Process created"),
        "Failed to create {}: {}",
        name2,
        output
    );
    let process_id2 = extract_process_id(&output)
        .expect("Failed to extract process ID")
        .to_string();

    // Start both processes
    run_cli(&["start", &process_id1]);
    run_cli(&["start", &process_id2]);
    thread::sleep(Duration::from_secs(1));

    // Verify both PID files exist
    assert!(Path::new(&pidfile1).exists(), "PID file 1 should exist");
    assert!(Path::new(&pidfile2).exists(), "PID file 2 should exist");

    // Read and verify different PIDs
    let pid1: u32 = fs::read_to_string(&pidfile1)
        .unwrap()
        .trim()
        .parse()
        .unwrap();
    let pid2: u32 = fs::read_to_string(&pidfile2)
        .unwrap()
        .trim()
        .parse()
        .unwrap();
    assert_ne!(pid1, pid2, "PIDs should be different");

    // Stop first process
    run_cli(&["stop", &process_id1]);
    thread::sleep(Duration::from_secs(1));

    // Verify only first PID file is removed
    assert!(
        !Path::new(&pidfile1).exists(),
        "PID file 1 should be removed"
    );
    assert!(
        Path::new(&pidfile2).exists(),
        "PID file 2 should still exist"
    );

    // Stop second process
    run_cli(&["stop", &process_id2]);
    thread::sleep(Duration::from_secs(1));

    // Verify second PID file is removed
    assert!(
        !Path::new(&pidfile2).exists(),
        "PID file 2 should be removed"
    );

    // Daemon cleanup handled by guard
}
