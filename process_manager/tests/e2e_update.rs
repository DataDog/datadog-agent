//! E2E tests for the update command

mod common;

use common::*;

#[test]
fn test_update_restart_policy_hot_update() {
    let _daemon = setup_daemon();

    // Create and start a process (already waits for running state)
    let name = unique_process_name();
    let id = create_and_start_process(&name, "sleep", &["300"]);

    // Update restart policy (hot update, no restart required)
    let output = run_cli(&["update", &id, "--restart", "always"]);
    assert!(output.contains("[OK]"), "Update should succeed");
    assert!(
        output.contains("restart_policy"),
        "Should show updated field"
    );
    assert!(
        !output.contains("require restart"),
        "Should not require restart"
    );

    // Verify the update
    let desc = run_cli(&["describe", &id]);
    assert!(desc.contains("always"), "Restart policy should be updated");

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_update_env_requires_restart() {
    let _daemon = setup_daemon();

    // Create and start a process (already waits for running state)
    let name = unique_process_name();
    let id = create_and_start_process(&name, "sleep", &["300"]);

    // Update environment variable (requires restart)
    let output = run_cli(&["update", &id, "--env", "TEST_VAR=new_value"]);
    assert!(output.contains("[OK]"), "Update should succeed");
    assert!(output.contains("env"), "Should show updated field");
    assert!(
        output.contains("require restart"),
        "Should indicate restart required"
    );
    assert!(
        !output.contains("restarted successfully"),
        "Should not auto-restart"
    );

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_update_with_restart_process_flag() {
    let _daemon = setup_daemon();

    // Create and start a process (already waits for running state)
    let name = unique_process_name();
    let id = create_and_start_process(&name, "sleep", &["300"]);

    // Get initial PID
    let desc1 = run_cli(&["describe", &id]);
    let pid1 = desc1
        .lines()
        .find(|l| l.contains("PID:"))
        .and_then(|l| l.split_whitespace().nth(1))
        .and_then(|s| s.parse::<u32>().ok());

    // Update with restart flag
    let output = run_cli(&["update", &id, "--working-dir", "/tmp", "--restart-process"]);
    assert!(output.contains("[OK]"), "Update should succeed");
    assert!(output.contains("working_dir"), "Should show updated field");
    assert!(
        output.contains("restarted successfully"),
        "Should indicate restart happened"
    );

    // Wait for restart
    std::thread::sleep(std::time::Duration::from_secs(2));

    // Verify PID changed
    let desc2 = run_cli(&["describe", &id]);
    let pid2 = desc2
        .lines()
        .find(|l| l.contains("PID:"))
        .and_then(|l| l.split_whitespace().nth(1))
        .and_then(|s| s.parse::<u32>().ok());

    if let (Some(p1), Some(p2)) = (pid1, pid2) {
        assert_ne!(p1, p2, "PID should have changed after restart");
    }

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_update_multiple_fields() {
    let _daemon = setup_daemon();

    // Create and start a process with unique name
    let name = unique_process_name();
    let id = create_and_start_process(&name, "sleep", &["300"]);

    // Update multiple fields at once
    let output = run_cli(&[
        "update",
        &id,
        "--restart",
        "on-failure",
        "--timeout-stop-sec",
        "15",
        "--restart-sec",
        "5",
    ]);
    assert!(output.contains("[OK]"), "Update should succeed");
    assert!(
        output.contains("restart_policy"),
        "Should update restart policy"
    );
    assert!(
        output.contains("timeout_stop_sec"),
        "Should update timeout_stop_sec"
    );
    assert!(output.contains("restart_sec"), "Should update restart_sec");

    // Verify restart policy update
    let desc = run_cli(&["describe", &id]);
    assert!(
        desc.contains("on-failure"),
        "Restart policy should be updated"
    );
    // Note: timeout_stop_sec, restart_sec may not appear in describe output
    // if they're not shown in the CONFIGURATION section, but the update succeeded

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_update_nonexistent_process() {
    let _daemon = setup_daemon();

    // Try to update non-existent process
    let (stdout, stderr, exit_code) =
        run_cli_full(&["update", "nonexistent-id", "--restart", "always"]);
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]") || exit_code != 0,
        "Should fail for non-existent process. Exit code: {}, Output: {}",
        exit_code,
        output
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_update_invalid_restart_policy() {
    let _daemon = setup_daemon();

    // Create a process with unique name
    let name = unique_process_name();
    let id = create_process(&name, "sleep", &["300"]);

    // Try invalid restart policy
    let (stdout, stderr, exit_code) = run_cli_full(&["update", &id, "--restart", "invalid-policy"]);
    let output = format!("{}{}", stdout, stderr);
    assert!(
        output.contains("[ERROR]") || output.contains("Invalid") || exit_code != 0,
        "Should reject invalid restart policy. Exit code: {}, Output: {}",
        exit_code,
        output
    );

    // Cleanup
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_update_resource_limits() {
    let _daemon = setup_daemon();

    // Create and start a process (already waits for running state)
    let name = unique_process_name();
    let id = create_and_start_process(&name, "sleep", &["300"]);

    // Update CPU limit (hot update)
    let output = run_cli(&["update", &id, "--cpu-limit", "2"]);
    assert!(output.contains("[OK]"), "Update should succeed");
    assert!(
        output.contains("resources"),
        "Should show updated resources"
    );

    // Update memory limit
    let output = run_cli(&["update", &id, "--memory-limit", "512M"]);
    assert!(output.contains("[OK]"), "Update should succeed");

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_update_dry_run() {
    let _daemon = setup_daemon();

    // Create a process
    let name = unique_process_name();
    let id = create_process(&name, "sleep", &["300"]);

    // Dry run - should not apply changes
    let output = run_cli(&["update", &id, "--restart", "always", "--dry-run"]);
    assert!(output.contains("OK"), "Dry run should succeed");
    assert!(output.contains("Dry run"), "Should indicate it's a dry run");

    // Verify nothing changed
    let desc = run_cli(&["describe", &id]);
    assert!(!desc.contains("always"), "Should not have applied change");

    // Cleanup
    delete_process(&id);
    // Daemon cleanup handled by guard
}
