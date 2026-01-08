//! E2E tests for auto_start functionality

use pm_e2e_tests::{create_process, delete_process, run_cli_full, setup_daemon};

fn unique_name(base: &str) -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_micros();
    format!("{}-{}", base, timestamp % 1000000)
}

#[test]
fn test_e2e_auto_start_false_returns_created_state() {
    let _daemon = setup_daemon();

    // Create process without auto_start
    let name = unique_name("test-no-auto-start");
    let id = create_process(&name, "sleep", &["10"]);

    // Verify process is in Created state
    let (stdout, _, _) = run_cli_full(&["describe", &id]);
    assert!(
        stdout.to_lowercase().contains("state") && stdout.to_lowercase().contains("created"),
        "Process should be in Created state, got: {}",
        stdout
    );
    assert!(
        stdout.contains("PID:             -") || stdout.contains("pid:             -"),
        "Process should not have a PID"
    );

    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_auto_start_true_returns_running_state() {
    let _daemon = setup_daemon();

    let name = unique_name("test-auto-start");

    // Create process with --auto-start
    let (stdout, stderr, exit_code) =
        run_cli_full(&["create", &name, "sleep", "60", "--auto-start"]);

    assert_eq!(
        exit_code, 0,
        "Create with auto-start should succeed: {}",
        stderr
    );
    assert!(
        stdout.contains("Process created") || stdout.contains("Auto-started"),
        "Should indicate process was created/started, got: {}",
        stdout
    );

    // Extract process ID
    let id = pm_e2e_tests::extract_process_id(&stdout)
        .expect("Should extract process ID")
        .to_string();

    // Verify process is in Running state
    let (stdout, _, _) = run_cli_full(&["describe", &id]);
    let stdout_lower = stdout.to_lowercase();
    assert!(
        stdout_lower.contains("running"),
        "Process should be in Running state when auto_start=true, got: {}",
        stdout
    );
    // Check PID is not empty (should have a number, not just "-")
    assert!(
        stdout.contains("PID:") && !stdout.contains("PID:             -"),
        "Process should have a PID, got: {}",
        stdout
    );

    // Stop and cleanup
    pm_e2e_tests::stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_auto_start_with_dependencies() {
    let _daemon = setup_daemon();

    let dep_name = unique_name("dep-service");
    let main_name = unique_name("main-service");

    // Create dependency first (no auto_start)
    let dep_id = create_process(&dep_name, "sleep", &["60"]);

    // Create main process with auto_start and dependency
    let (stdout, stderr, exit_code) = run_cli_full(&[
        "create",
        &main_name,
        "sleep",
        "60",
        "--requires",
        &dep_name,
        "--auto-start",
    ]);

    assert_eq!(exit_code, 0, "Create should succeed: {}", stderr);

    let main_id = pm_e2e_tests::extract_process_id(&stdout)
        .expect("Should extract process ID")
        .to_string();

    // Both should be running (auto_start triggers dependency resolution)
    let (main_stdout, _, _) = run_cli_full(&["describe", &main_id]);
    assert!(
        main_stdout.to_lowercase().contains("running"),
        "Main service should be running, got: {}",
        main_stdout
    );

    let (dep_stdout, _, _) = run_cli_full(&["describe", &dep_id]);
    assert!(
        dep_stdout.to_lowercase().contains("running"),
        "Dependency should be auto-started, got: {}",
        dep_stdout
    );

    // Cleanup
    pm_e2e_tests::stop_process(&main_id);
    pm_e2e_tests::stop_process(&dep_id);
    delete_process(&main_id);
    delete_process(&dep_id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_auto_start_with_restart_policy() {
    let _daemon = setup_daemon();

    let name = unique_name("test-restart-auto");

    // Create process with auto_start and restart policy
    let (stdout, stderr, exit_code) = run_cli_full(&[
        "create",
        &name,
        "sleep",
        "60",
        "--restart",
        "always",
        "--auto-start",
    ]);

    assert_eq!(exit_code, 0, "Create should succeed: {}", stderr);

    let id = pm_e2e_tests::extract_process_id(&stdout)
        .expect("Should extract process ID")
        .to_string();

    // Verify process is running
    let (stdout, _, _) = run_cli_full(&["describe", &id]);
    assert!(
        stdout.to_lowercase().contains("running"),
        "Process should be running, got: {}",
        stdout
    );
    assert!(
        stdout.to_lowercase().contains("always"),
        "Restart policy should be set to always, got: {}",
        stdout
    );

    // Cleanup
    pm_e2e_tests::stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_auto_start_with_oneshot_type() {
    let _daemon = setup_daemon();

    let name = unique_name("test-oneshot-auto");

    // Oneshot process with auto_start should complete immediately
    let (stdout, stderr, exit_code) = run_cli_full(&[
        "create",
        &name,
        "echo",
        "hello",
        "--process-type",
        "oneshot",
        "--auto-start",
    ]);

    assert_eq!(
        exit_code, 0,
        "Create should succeed. stdout: {}, stderr: {}",
        stdout, stderr
    );

    // Extract ID - oneshot completes quickly so output might be different
    let id = if let Some(id) = pm_e2e_tests::extract_process_id(&stdout) {
        id.to_string()
    } else {
        // Try alternate parsing if standard extraction fails
        panic!(
            "Could not extract process ID. stdout: {}, stderr: {}",
            stdout, stderr
        );
    };

    // Give it a moment to complete
    std::thread::sleep(std::time::Duration::from_millis(300));

    // Should be in Exited state (completed successfully)
    let (stdout, _, _) = run_cli_full(&["describe", &id]);
    assert!(
        stdout.to_lowercase().contains("exited"),
        "Oneshot process should be in Exited state after completion, got: {}",
        stdout
    );

    delete_process(&id);
    // Daemon cleanup handled by guard
}
