use pm_e2e_tests::{
    assert_process_state_by_name, create_process, run_cli, setup_daemon, start_process,
    stop_process, unique_process_name,
};
use std::time::Duration;

/// Test basic BindsTo: when main service stops, bound service also stops
#[test]
fn test_binds_to_cascade_stop() {
    let _daemon = setup_daemon();

    // Create main service (e.g., main agent)
    create_process("main-agent", "sleep", &["3600"]);

    // Create bound service (e.g., trace agent that binds to main agent)
    create_process(
        "trace-agent",
        "sleep",
        &["3600", "--binds-to", "main-agent"],
    );

    // Start both services
    start_process("main-agent");
    start_process("trace-agent");

    // Give processes time to start
    std::thread::sleep(Duration::from_millis(500));

    // Verify both are running
    assert!(
        assert_process_state_by_name("main-agent", "running"),
        "main-agent should be running"
    );
    assert!(
        assert_process_state_by_name("trace-agent", "running"),
        "trace-agent should be running"
    );

    // Stop main agent
    stop_process("main-agent");

    // Give cascade time to trigger
    std::thread::sleep(Duration::from_secs(2));

    // Verify both are stopped (trace-agent should have cascaded)
    let output = run_cli(&["list"]);
    assert!(
        !output.contains("main-agent")
            || (!output.contains("running") && output.contains("main-agent")),
        "main-agent should be stopped. output: {}",
        output
    );
    assert!(
        !output.contains("trace-agent")
            || (!output.contains("running") && output.contains("trace-agent")),
        "trace-agent should have been stopped by cascade. output: {}",
        output
    );

    // Daemon cleanup handled by guard
}

/// Test multiple services binding to the same main service
#[test]
fn test_binds_to_multiple_dependents() {
    let _daemon = setup_daemon();

    // Create main service
    create_process("main-service", "sleep", &["3600"]);

    // Create multiple bound services
    for service_name in &["dependent-1", "dependent-2", "dependent-3"] {
        create_process(
            service_name,
            "sleep",
            &["3600", "--binds-to", "main-service"],
        );
    }

    // Start all services
    start_process("main-service");
    for service_name in &["dependent-1", "dependent-2", "dependent-3"] {
        start_process(service_name);
    }

    // Give processes time to start
    std::thread::sleep(Duration::from_millis(500));

    // Verify all are running
    for service_name in &["main-service", "dependent-1", "dependent-2", "dependent-3"] {
        assert!(
            assert_process_state_by_name(service_name, "running"),
            "{} should be running",
            service_name
        );
    }

    // Stop main service
    stop_process("main-service");

    // Give cascade time to trigger
    std::thread::sleep(Duration::from_secs(2));

    // Verify all dependents were stopped
    let output = run_cli(&["list"]);
    for service_name in &["dependent-1", "dependent-2", "dependent-3"] {
        let is_stopped = !output.contains(service_name)
            || (!output.contains("running") && output.contains(service_name));
        assert!(
            is_stopped,
            "{} should have been stopped by cascade. output: {}",
            service_name, output
        );
    }

    // Daemon cleanup handled by guard
}

/// Test BindsTo cascade on process crash (not just manual stop)
#[test]
fn test_binds_to_cascade_on_crash() {
    let _daemon = setup_daemon();

    let crasher_name = unique_process_name();
    let bound_name = unique_process_name();

    // Create main service that will crash
    create_process(&crasher_name, "sh", &["-c", "sleep 1 && exit 1"]);

    // Create bound service
    create_process(&bound_name, "sleep", &["3600", "--binds-to", &crasher_name]);

    // Start both
    start_process(&crasher_name);
    start_process(&bound_name);

    // Give processes time to start and crasher to crash
    std::thread::sleep(Duration::from_secs(3));

    // Verify bound service was stopped when crasher crashed
    let output = run_cli(&["list"]);
    let bound_stopped = !output.contains(&bound_name)
        || (!output.contains("running") && output.contains(&bound_name));
    assert!(
        bound_stopped,
        "bound-service should have been stopped when crasher crashed. output: {}",
        output
    );

    // Daemon cleanup handled by guard
}

/// Test that BindsTo is listed in describe output
#[test]
fn test_binds_to_in_describe() {
    let _daemon = setup_daemon();

    let main_name = unique_process_name();
    let dependent_name = unique_process_name();

    // Create main service
    create_process(&main_name, "sleep", &["3600"]);

    // Create bound service
    create_process(
        &dependent_name,
        "sleep",
        &["3600", "--binds-to", &main_name],
    );

    // Describe the dependent service
    let output = run_cli(&["describe", &dependent_name]);
    assert!(
        output.contains("BindsTo"),
        "describe output should show BindsTo dependency. output: {}",
        output
    );
    assert!(
        output.contains(&main_name),
        "describe output should show the main dependency. output: {}",
        output
    );

    // Daemon cleanup handled by guard
}

/// Test process without BindsTo dependency works normally
#[test]
fn test_no_binds_to_works_normally() {
    let _daemon = setup_daemon();

    // Create two independent services
    create_process("service-a", "sleep", &["3600"]);
    create_process("service-b", "sleep", &["3600"]);

    // Start both
    start_process("service-a");
    start_process("service-b");

    // Give processes time to start
    std::thread::sleep(Duration::from_millis(500));

    // Verify both are running
    assert!(
        assert_process_state_by_name("service-a", "running"),
        "service-a should be running"
    );
    assert!(
        assert_process_state_by_name("service-b", "running"),
        "service-b should be running"
    );

    // Stop service-a
    stop_process("service-a");

    // Give time for any potential cascade (should not happen)
    std::thread::sleep(Duration::from_secs(1));

    // Verify service-b is still running (no cascade)
    assert!(
        assert_process_state_by_name("service-b", "running"),
        "service-b should still be running (no BindsTo)"
    );

    // Daemon cleanup handled by guard
}
