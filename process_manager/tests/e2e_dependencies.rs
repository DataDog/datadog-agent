/// E2E tests for process dependencies
use pm_e2e_tests::{
    create_process, delete_process, run_cli, setup_daemon, start_process, stop_process,
    wait_for_state,
};
use std::thread::sleep;
use std::time::Duration;

#[test]
fn test_e2e_requires_auto_start() {
    let _daemon = setup_daemon();

    // Create base process
    let base_id = create_process("e2e-dep-base", "sleep", &["30"]);

    // Create dependent process with --requires
    let app_id = create_process(
        "e2e-dep-app",
        "sleep",
        &[
            "30",
            "--requires",
            "e2e-dep-base",
            "--after",
            "e2e-dep-base",
        ],
    );

    // Start only the dependent - should auto-start base
    start_process(&app_id);
    sleep(Duration::from_secs(3));

    // Both should be running
    assert!(
        wait_for_state(&base_id, "running", 5),
        "Base should be auto-started"
    );
    assert!(
        wait_for_state(&app_id, "running", 5),
        "App should be running"
    );

    // Clean up
    stop_process(&app_id);
    stop_process(&base_id);
    sleep(Duration::from_secs(1));
    delete_process(&app_id);
    delete_process(&base_id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_wants_soft_dependency() {
    let _daemon = setup_daemon();

    // Create base process
    let base_id = create_process("e2e-wants-base", "sleep", &["30"]);

    // Create dependent with --wants
    let app_id = create_process(
        "e2e-wants-app",
        "sleep",
        &[
            "30",
            "--wants",
            "e2e-wants-base",
            "--after",
            "e2e-wants-base",
        ],
    );

    // Start the app
    start_process(&app_id);
    sleep(Duration::from_secs(3));

    // Both should be running (wants also auto-starts)
    assert!(
        wait_for_state(&app_id, "running", 5),
        "App should be running"
    );

    // Clean up
    stop_process(&app_id);
    stop_process(&base_id);
    sleep(Duration::from_secs(1));
    delete_process(&app_id);
    delete_process(&base_id);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_dependency_chain() {
    let _daemon = setup_daemon();

    // Create chain: A -> B -> C
    let id_a = create_process("e2e-chain-a", "sleep", &["30"]);
    let id_b = create_process(
        "e2e-chain-b",
        "sleep",
        &["30", "--requires", "e2e-chain-a", "--after", "e2e-chain-a"],
    );
    let id_c = create_process(
        "e2e-chain-c",
        "sleep",
        &["30", "--requires", "e2e-chain-b", "--after", "e2e-chain-b"],
    );

    // Start only C - should cascade
    start_process(&id_c);
    sleep(Duration::from_secs(4));

    // All should be running
    assert!(wait_for_state(&id_a, "running", 5), "A should be running");
    assert!(wait_for_state(&id_b, "running", 5), "B should be running");
    assert!(wait_for_state(&id_c, "running", 5), "C should be running");

    // Clean up
    stop_process(&id_c);
    stop_process(&id_b);
    stop_process(&id_a);
    sleep(Duration::from_secs(1));
    delete_process(&id_c);
    delete_process(&id_b);
    delete_process(&id_a);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_describe_shows_dependencies() {
    let _daemon = setup_daemon();

    // Create a process with dependencies
    let id = create_process(
        "e2e-dep-info",
        "sleep",
        &[
            "10",
            "--requires",
            "dep1",
            "--wants",
            "dep2",
            "--after",
            "dep3",
        ],
    );

    // Describe should show dependencies
    let describe_output = run_cli(&["describe", &id]);
    println!("Describe output: {}", describe_output);

    assert!(
        describe_output.contains("dep1"),
        "Should show requires dependency"
    );
    assert!(
        describe_output.contains("dep2"),
        "Should show wants dependency"
    );
    assert!(
        describe_output.contains("dep3"),
        "Should show after dependency"
    );

    // Clean up
    delete_process(&id);

    // Daemon cleanup handled by guard
}
