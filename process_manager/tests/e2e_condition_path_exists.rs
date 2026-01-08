use pm_e2e_tests::{
    create_process, run_cli, run_cli_full, setup_daemon, start_process, stop_process,
    unique_process_name,
};

use std::fs;
use tempfile::TempDir;

/// Test AND logic (default): all paths must exist
#[test]
fn test_condition_path_exists_and_logic() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let test_file1 = temp_dir.path().join("config1.yaml");
    let test_file2 = temp_dir.path().join("config2.yaml");

    // Create both files
    fs::write(&test_file1, "test config 1").expect("Failed to write test file 1");
    fs::write(&test_file2, "test config 2").expect("Failed to write test file 2");

    let _daemon = setup_daemon();

    // Create process with two AND conditions (both must exist)
    let process_name = unique_process_name();
    create_process(
        &process_name,
        "sleep",
        &[
            "10",
            "--condition-path-exists",
            test_file1.to_str().unwrap(),
            "--condition-path-exists",
            test_file2.to_str().unwrap(),
        ],
    );

    // Start should succeed because both files exist
    start_process(&process_name);

    // Stop process
    stop_process(&process_name);

    // Now remove one file
    fs::remove_file(&test_file1).expect("Failed to remove test file");

    // Start should fail because one condition is not met
    let (stdout, stderr, exit_code) = run_cli_full(&["start", &process_name]);
    assert_ne!(exit_code, 0, "Start should fail when condition not met");
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]")
            || output.contains("Condition failed")
            || output.contains("path must exist"),
        "Process should not start when AND condition fails. output: {}",
        output
    );

    // Daemon cleanup handled by guard
}

/// Test NOT logic (!): path must NOT exist
#[test]
fn test_condition_path_exists_not_logic() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let override_file = temp_dir.path().join("override.conf");

    let _daemon = setup_daemon();

    // Create process that should NOT start if override file exists
    let process_name = unique_process_name();
    let override_condition = format!("!{}", override_file.to_str().unwrap());
    create_process(
        &process_name,
        "sleep",
        &["10", "--condition-path-exists", &override_condition],
    );

    // Start should succeed because file does NOT exist
    start_process(&process_name);

    // Stop process
    stop_process(&process_name);

    // Now create the file
    fs::write(&override_file, "override config").expect("Failed to write override file");

    // Start should fail because file now exists (violates NOT condition)
    let (stdout, stderr, exit_code) = run_cli_full(&["start", &process_name]);
    assert_ne!(
        exit_code, 0,
        "Start should fail when NOT condition violated"
    );
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]")
            || output.contains("Condition failed")
            || output.contains("must NOT exist"),
        "Process should not start when NOT condition fails (file exists). output: {}",
        output
    );

    // Daemon cleanup handled by guard
}

/// Test OR logic (|): at least one path must exist
#[test]
fn test_condition_path_exists_or_logic() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let config_default = temp_dir.path().join("config.yaml");
    let config_managed = temp_dir.path().join("managed/config.yaml");

    // Create managed directory
    fs::create_dir_all(config_managed.parent().unwrap()).expect("Failed to create managed dir");

    let _daemon = setup_daemon();

    // Create process with two OR conditions (at least one must exist)
    let process_name = unique_process_name();
    let default_or_condition = format!("|{}", config_default.to_str().unwrap());
    let managed_or_condition = format!("|{}", config_managed.to_str().unwrap());
    create_process(
        &process_name,
        "sleep",
        &[
            "10",
            "--condition-path-exists",
            &default_or_condition,
            "--condition-path-exists",
            &managed_or_condition,
        ],
    );

    // Start should fail because neither file exists
    let (stdout, stderr, exit_code) = run_cli_full(&["start", &process_name]);
    assert_ne!(
        exit_code, 0,
        "Start should fail when no OR conditions satisfied"
    );
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]") || output.contains("Condition failed"),
        "Process should not start when no OR conditions satisfied. output: {}",
        output
    );

    // Create one of the files (default location)
    fs::write(&config_default, "default config").expect("Failed to write default config");

    // Start should succeed because at least one OR condition is satisfied
    start_process(&process_name);

    // Stop process
    stop_process(&process_name);

    // Remove default, create managed
    fs::remove_file(&config_default).expect("Failed to remove default config");
    fs::write(&config_managed, "managed config").expect("Failed to write managed config");

    // Start should still succeed because the other OR condition is satisfied
    start_process(&process_name);

    // Daemon cleanup handled by guard
}

/// Test mixed AND and OR logic
#[test]
fn test_condition_path_exists_mixed_logic() {
    let temp_dir = TempDir::new().expect("Failed to create temp dir");
    let required_binary = temp_dir.path().join("bin/app");
    let override_file = temp_dir.path().join("override.conf");
    let config_default = temp_dir.path().join("config.yaml");
    let config_managed = temp_dir.path().join("managed/config.yaml");

    // Create directories
    fs::create_dir_all(required_binary.parent().unwrap()).expect("Failed to create bin dir");
    fs::create_dir_all(config_managed.parent().unwrap()).expect("Failed to create managed dir");

    let _daemon = setup_daemon();

    // Create process with mixed conditions:
    // - Binary must exist (AND)
    // - Override must NOT exist (AND with NOT)
    // - At least one config must exist (OR)
    let process_name = unique_process_name();
    let not_override_condition = format!("!{}", override_file.to_str().unwrap());
    let default_or_condition = format!("|{}", config_default.to_str().unwrap());
    let managed_or_condition = format!("|{}", config_managed.to_str().unwrap());
    create_process(
        &process_name,
        "sleep",
        &[
            "10",
            "--condition-path-exists",
            required_binary.to_str().unwrap(),
            "--condition-path-exists",
            &not_override_condition,
            "--condition-path-exists",
            &default_or_condition,
            "--condition-path-exists",
            &managed_or_condition,
        ],
    );

    // Should fail: binary doesn't exist yet
    let (stdout, stderr, exit_code) = run_cli_full(&["start", &process_name]);
    assert_ne!(exit_code, 0, "Start should fail when AND condition not met");
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]") || output.contains("Condition failed"),
        "Process should not start when AND condition not met. output: {}",
        output
    );

    // Create required binary
    fs::write(&required_binary, "#!/bin/sh\necho test").expect("Failed to write binary");

    // Should fail: no config exists
    let (stdout, stderr, exit_code) = run_cli_full(&["start", &process_name]);
    assert_ne!(exit_code, 0, "Start should fail when OR conditions not met");
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]") || output.contains("Condition failed"),
        "Process should not start when OR conditions not met. output: {}",
        output
    );

    // Create one config (satisfies OR)
    fs::write(&config_default, "config").expect("Failed to write config");

    // Should succeed: all conditions met
    start_process(&process_name);

    // Stop process
    stop_process(&process_name);

    // Create override file (violates NOT condition)
    fs::write(&override_file, "override").expect("Failed to write override");

    // Should fail: NOT condition violated
    let (stdout, stderr, exit_code) = run_cli_full(&["start", &process_name]);
    assert_ne!(
        exit_code, 0,
        "Start should fail when NOT condition violated"
    );
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]") || output.contains("Condition failed"),
        "Process should not start when NOT condition violated. output: {}",
        output
    );

    // Daemon cleanup handled by guard
}

/// Test that process without conditions works normally
#[test]
fn test_no_conditions_works_normally() {
    let _daemon = setup_daemon();

    // Create process without any conditions (with auto-start)
    let process_name = unique_process_name();
    create_process(&process_name, "echo", &["hello", "--auto-start"]);

    // Process should start successfully without any condition checks
    std::thread::sleep(std::time::Duration::from_millis(200));

    let output = run_cli(&["list"]);
    assert!(
        output.contains(&process_name),
        "Process should be in list. output: {}",
        output
    );

    // Daemon cleanup handled by guard
}
