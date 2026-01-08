/// End-to-end tests using daemon + CLI
use pm_e2e_tests::{
    assert_process_exit_code_by_id, assert_process_state_by_id, assert_process_state_by_name,
    create_and_start_process, create_process, delete_process, process_exists_by_id,
    process_exists_by_name, run_cli, setup_daemon, stop_process, unique_process_name,
};
use std::thread::sleep;
use std::time::Duration;

#[test]
fn test_e2e_create_list_delete() {
    let _daemon = setup_daemon();

    let process_name = unique_process_name();

    // Create a process
    let id = create_process(&process_name, "sleep", &["100"]);

    // Verify process is listed and in 'created' state (not started yet)
    assert!(
        assert_process_state_by_name(&process_name, "created"),
        "Process should be in 'created' state by name"
    );
    assert!(
        assert_process_state_by_id(&id, "created"),
        "Process should be in 'created' state by ID"
    );

    // Delete process
    delete_process(&id);

    // Verify it's gone
    assert!(
        !process_exists_by_name(&process_name),
        "Process should not appear in list after deletion (by name)"
    );
    assert!(
        !process_exists_by_id(&id),
        "Process should not appear in list after deletion (by ID)"
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_start_stop() {
    let _daemon = setup_daemon();

    let process_name = unique_process_name();
    // Create and start a long-running process
    let id = create_and_start_process(&process_name, "sleep", &["60"]);

    // Stop the process (waits for stopped state)
    stop_process(&id);

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_describe() {
    let _daemon = setup_daemon();

    let process_name = unique_process_name();
    // Create a process
    let id = create_process(&process_name, "echo", &["hello"]);

    // Describe the process
    let describe_output = run_cli(&["describe", &id]);
    println!("Describe output: {}", describe_output);

    // Verify describe contains key information
    assert!(
        describe_output.contains(&process_name),
        "Should contain process name"
    );
    assert!(describe_output.contains(&id), "Should contain process ID");
    assert!(describe_output.contains("echo"), "Should contain command");
    assert!(describe_output.contains("created"), "Should contain state");

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_restart_policy_in_create() {
    let _daemon = setup_daemon();

    let process_name = unique_process_name();
    // Create a process with restart policy
    let id = create_process(&process_name, "sleep", &["1", "--restart", "always"]);

    // Verify in describe
    let describe_output = run_cli(&["describe", &id]);
    assert!(
        describe_output.contains("always"),
        "Restart policy should be 'always'"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_force_delete() {
    let _daemon = setup_daemon();

    // Create and start a process
    let process_name = unique_process_name();
    let id = create_and_start_process(&process_name, "sleep", &["100"]);
    sleep(Duration::from_secs(1));

    // Force delete while running
    delete_process(&id); // delete_process already uses --force

    // Give it a moment to fully delete
    sleep(Duration::from_millis(500));

    // Verify it's gone
    let list_output = run_cli(&["list"]);
    assert!(
        !process_exists_by_name(&process_name),
        "Process should not appear after force delete (by name). List output:\n{}",
        list_output
    );
    assert!(
        !process_exists_by_id(&id),
        "Process should not appear after force delete (by ID)"
    );

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_process_exit_success() {
    let _daemon = setup_daemon();

    let process_name = unique_process_name();

    // Create and start a process that exits with code 0 (success)
    // Don't wait for "running" since it exits immediately
    let id = create_process(&process_name, "bash", &["-c", "exit 0"]);
    let _ = run_cli(&["start", &id]); // Start without waiting for "running" state
    sleep(Duration::from_secs(2));

    // Check state
    assert!(
        assert_process_state_by_name(&process_name, "exited"),
        "Process should have exited"
    );
    assert!(
        assert_process_state_by_id(&id, "exited"),
        "Process should have exited"
    );

    // Check exit code is 0
    assert!(
        assert_process_exit_code_by_id(&id, 0),
        "Should have exit code 0"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_process_exit_failure() {
    let _daemon = setup_daemon();

    let process_name = unique_process_name();

    // Create and start a process that exits with code 1 (failure)
    // Don't wait for "running" since it exits immediately
    let id = create_process(&process_name, "bash", &["-c", "exit 1"]);
    let _ = run_cli(&["start", &id]); // Start without waiting for "running" state
    sleep(Duration::from_secs(2));

    // Check state (should be "crashed" for non-zero exit)
    assert!(
        assert_process_state_by_name(&process_name, "crashed"),
        "Process should have crashed (non-zero exit)"
    );
    assert!(
        assert_process_state_by_id(&id, "crashed"),
        "Process should have crashed (non-zero exit)"
    );

    // Check exit code is 1
    assert!(
        assert_process_exit_code_by_id(&id, 1),
        "Should have exit code 1"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}
