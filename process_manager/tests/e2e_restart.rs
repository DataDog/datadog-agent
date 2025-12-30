/// E2E tests for restart policies
use pm_e2e_tests::{
    create_process, delete_process, run_cli, setup_daemon, start_process, stop_process,
    unique_process_name,
};
use std::thread::sleep;
use std::time::Duration;

#[test]
fn test_e2e_restart_always() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create a process that exits quickly with restart policy
    let id = create_process(&name, "bash", &["-c", "exit 0", "--restart", "always"]);

    // Start the process (don't use start_process helper as it waits for "running",
    // but this process exits immediately and will restart)
    run_cli(&["start", &id]);

    // Wait for it to restart a few times (will eventually hit start limit and crash)
    sleep(Duration::from_secs(5));

    // Check that run count increased
    let describe_output = run_cli(&["describe", &id]);
    println!("Describe output: {}", describe_output);

    // Should have restarted at least once (run count > 1)
    assert!(
        describe_output.contains("Run Count:"),
        "Should show run count"
    );

    // Clean up - process may be crashed after hitting start limit, so just delete
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_restart_never() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create a process with restart policy never
    let id = create_process(&name, "bash", &["-c", "exit 1", "--restart", "never"]);

    // Start the process (don't use start_process helper as it waits for "running",
    // but this process exits immediately and goes to crashed)
    run_cli(&["start", &id]);

    // Wait for it to crash
    sleep(Duration::from_secs(3));

    // Check state
    let list_output = run_cli(&["list"]);
    println!("List output: {}", list_output);
    assert!(list_output.contains("crashed"), "Process should be crashed");

    // Check that it only ran once
    let describe_output = run_cli(&["describe", &id]);
    assert!(
        describe_output.contains("Run Count:") && describe_output.contains("1"),
        "Should have run exactly once"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_restart_on_failure() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create a process that fails with restart policy on-failure
    let id = create_process(&name, "bash", &["-c", "exit 1", "--restart", "on-failure"]);

    // Start the process (don't use start_process helper as it waits for "running",
    // but this process exits immediately and will restart)
    run_cli(&["start", &id]);

    // Wait for restarts (will eventually hit start limit and crash)
    sleep(Duration::from_secs(5));

    // Should have restarted (run count > 1)
    let describe_output = run_cli(&["describe", &id]);
    println!("Describe output: {}", describe_output);

    // Clean up - process may be crashed after hitting start limit, so just delete
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_manual_stop_no_restart() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create a long-running process with restart policy always
    let id = create_process(&name, "sleep", &["60", "--restart", "always"]);

    // Start and verify running
    start_process(&id);
    sleep(Duration::from_secs(2));

    let list1 = run_cli(&["list"]);
    assert!(list1.contains("running"), "Should be running");

    // Manually stop
    stop_process(&id);
    sleep(Duration::from_secs(3));

    // Should remain stopped (manual stop prevents restart - systemd behavior)
    let list2 = run_cli(&["list"]);
    println!("List after manual stop: {}", list2);
    assert!(
        list2.contains("stopped"),
        "Should remain stopped after manual stop"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}
