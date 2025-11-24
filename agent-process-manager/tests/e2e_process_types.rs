/// E2E tests for process types
use pm_e2e_tests::{
    create_process, delete_process, run_cli, setup_daemon, start_process, unique_process_name,
};
use std::thread::sleep;
use std::time::Duration;

#[test]
fn test_e2e_type_simple() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create simple type process
    let id = create_process(&name, "sleep", &["5", "--process-type", "simple"]);

    // Start it
    start_process(&id);
    sleep(Duration::from_secs(2));

    // Should be running immediately
    let list_output = run_cli(&["list"]);
    println!("Simple type list: {}", list_output);
    assert!(
        list_output.contains("running"),
        "Simple type should be running immediately"
    );

    // Wait for exit
    sleep(Duration::from_secs(4));
    let list2 = run_cli(&["list"]);
    assert!(
        list2.contains("exited"),
        "Should exit after sleep completes"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_type_oneshot_success() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create oneshot type process that succeeds
    let id = create_process(
        &name,
        "bash",
        &["-c", "exit 0", "--process-type", "oneshot"],
    );

    // Start it (don't use start_process helper as it waits for "running",
    // but oneshot processes exit immediately and go to "exited")
    run_cli(&["start", &id]);
    sleep(Duration::from_secs(2));

    // Should be exited (success)
    let list_output = run_cli(&["list"]);
    println!("Oneshot success list: {}", list_output);
    assert!(
        list_output.contains("exited"),
        "Oneshot should be exited after completion"
    );

    // Check exit code
    let describe_output = run_cli(&["describe", &id]);
    assert!(
        describe_output.contains("Exit Code:") && describe_output.contains("0"),
        "Should have exit code 0"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_type_oneshot_failure() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create oneshot type process that fails
    let id = create_process(
        &name,
        "bash",
        &["-c", "exit 1", "--process-type", "oneshot"],
    );

    // Start it (don't use start_process helper as it waits for "running",
    // but oneshot processes that fail immediately go to "crashed")
    run_cli(&["start", &id]);
    sleep(Duration::from_secs(2));

    // Should be crashed (failure)
    let list_output = run_cli(&["list"]);
    println!("Oneshot failure list: {}", list_output);
    assert!(
        list_output.contains("crashed"),
        "Oneshot should be crashed after failure"
    );

    // Check exit code
    let describe_output = run_cli(&["describe", &id]);
    assert!(
        describe_output.contains("Exit Code:") && describe_output.contains("1"),
        "Should have exit code 1"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_type_oneshot_no_restart() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create oneshot with restart policy (should be ignored)
    let id = create_process(
        &name,
        "bash",
        &[
            "-c",
            "exit 0",
            "--process-type",
            "oneshot",
            "--restart",
            "always",
        ],
    );

    // Start it (don't use start_process helper as it waits for "running",
    // but oneshot processes exit immediately and go to "exited")
    run_cli(&["start", &id]);
    sleep(Duration::from_secs(1));

    let describe1 = run_cli(&["describe", &id]);
    assert!(
        describe1.contains("Run Count:") && describe1.contains("1"),
        "Should run once"
    );

    // Wait longer - should NOT restart
    sleep(Duration::from_secs(4));

    let describe2 = run_cli(&["describe", &id]);
    println!("Oneshot no-restart describe: {}", describe2);
    assert!(
        describe2.contains("Run Count:") && describe2.contains("1"),
        "Oneshot should never restart, even with restart policy"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_type_default() {
    let _daemon = setup_daemon();

    let name = unique_process_name();

    // Create without specifying type (should default to simple)
    let id = create_process(&name, "sleep", &["3"]);

    // Start it
    start_process(&id);
    sleep(Duration::from_secs(1));

    // Should behave like simple (running immediately)
    let list_output = run_cli(&["list"]);
    println!("Default type list: {}", list_output);
    assert!(
        list_output.contains("running"),
        "Default should behave like simple type"
    );

    // Clean up
    sleep(Duration::from_secs(3));
    delete_process(&id);

    // Daemon cleanup handled by guard
}
