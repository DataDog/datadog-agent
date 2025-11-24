use pm_e2e_tests::{
    assert_process_state_by_name, create_and_start_process, create_process, run_cli, setup_daemon,
    start_process, stop_process, unique_process_name,
};

use std::time::Duration;

/// Test basic Conflicts: starting process stops conflicting process
#[test]
fn test_conflicts_stops_running_process() {
    let _daemon = setup_daemon();

    let stable_name = unique_process_name();
    let exp_name = unique_process_name();

    // Create experimental agent that conflicts with stable
    create_process(&exp_name, "sleep", &["3600", "--conflicts", &stable_name]);

    // Create and start stable agent
    let _id = create_and_start_process(&stable_name, "sleep", &["3600"]);

    // Give time to start
    std::thread::sleep(Duration::from_millis(500));

    // Verify stable is running
    assert!(
        assert_process_state_by_name(&stable_name, "running"),
        "Stable agent should be running"
    );

    // Start experimental agent - should STOP stable agent first, then start
    start_process(&exp_name);

    // Give time for conflict resolution and start
    std::thread::sleep(Duration::from_secs(1));

    // Verify stable is NO LONGER running (stopped by conflict)
    let output = run_cli(&["list"]);
    // Check that stable agent line doesn't contain "running"
    let stable_line = output
        .lines()
        .find(|line| line.contains(&stable_name) && !line.contains(&exp_name));
    if let Some(line) = stable_line {
        assert!(
            !line.contains("running"),
            "Stable agent should be stopped. line: {}",
            line
        );
    }

    // Verify experimental IS running
    assert!(
        assert_process_state_by_name(&exp_name, "running"),
        "Experimental agent should be running"
    );

    // Daemon cleanup handled by guard
}

/// Test conflicts with already-stopped process
#[test]
fn test_conflicts_with_stopped_process() {
    let _daemon = setup_daemon();

    // Create exp-agent with conflicts
    create_process(
        "exp-agent",
        "sleep",
        &["3600", "--conflicts", "stable-agent"],
    );

    // Create and start stable agent
    let _id = create_and_start_process("stable-agent", "sleep", &["3600"]);
    std::thread::sleep(Duration::from_millis(500));

    // Stop stable agent manually
    stop_process("stable-agent");
    std::thread::sleep(Duration::from_millis(500));

    // Now exp agent should start successfully (nothing to conflict with)
    start_process("exp-agent");

    std::thread::sleep(Duration::from_millis(500));

    // Verify exp agent is running
    assert!(
        assert_process_state_by_name("exp-agent", "running"),
        "Exp agent should be running"
    );

    // Daemon cleanup handled by guard
}

/// Test multiple conflicts - starting process stops ALL conflicting processes
#[test]
fn test_multiple_conflicts() {
    let _daemon = setup_daemon();

    // Create new service that conflicts with both
    create_process(
        "new-service",
        "sleep",
        &[
            "3600",
            "--conflicts",
            "base-service",
            "--conflicts",
            "alt-service",
        ],
    );

    // Create and start both base and alt services
    let _id1 = create_and_start_process("base-service", "sleep", &["3600"]);
    let _id2 = create_and_start_process("alt-service", "sleep", &["3600"]);

    std::thread::sleep(Duration::from_millis(500));

    // Verify both are running
    assert!(
        assert_process_state_by_name("base-service", "running"),
        "base-service should be running"
    );
    assert!(
        assert_process_state_by_name("alt-service", "running"),
        "alt-service should be running"
    );

    // Start new service - should STOP BOTH base and alt
    start_process("new-service");

    std::thread::sleep(Duration::from_secs(1));

    // Verify both base and alt are now stopped
    let output = run_cli(&["list"]);
    let base_line = output.lines().find(|line| line.contains("base-service"));
    if let Some(line) = base_line {
        assert!(
            !line.contains("running"),
            "base-service should be stopped. line: {}",
            line
        );
    }

    let alt_line = output.lines().find(|line| line.contains("alt-service"));
    if let Some(line) = alt_line {
        assert!(
            !line.contains("running"),
            "alt-service should be stopped. line: {}",
            line
        );
    }

    // Verify new service is running
    assert!(
        assert_process_state_by_name("new-service", "running"),
        "new-service should be running"
    );

    // Daemon cleanup handled by guard
}

/// Test bidirectional conflicts - demonstrates that conflicts work both ways
#[test]
fn test_bidirectional_conflicts() {
    let _daemon = setup_daemon();

    // Create service B that conflicts with A
    create_process("service-b", "sleep", &["3600", "--conflicts", "service-a"]);

    // Create and start service A (conflicts with B)
    let _id = create_and_start_process("service-a", "sleep", &["3600", "--conflicts", "service-b"]);

    std::thread::sleep(Duration::from_millis(500));

    // Verify A is running
    assert!(
        assert_process_state_by_name("service-a", "running"),
        "service-a should be running"
    );

    // Start service B - should STOP A first, then start B
    start_process("service-b");

    std::thread::sleep(Duration::from_secs(1));

    // Verify A is now stopped
    let output = run_cli(&["list"]);
    let service_a_line = output.lines().find(|line| line.contains("service-a"));
    if let Some(line) = service_a_line {
        assert!(
            !line.contains("running"),
            "service-a should be stopped. line: {}",
            line
        );
    }

    // Verify B is running
    assert!(
        assert_process_state_by_name("service-b", "running"),
        "service-b should be running"
    );

    // Start service A again - should STOP B first, then start A
    start_process("service-a");

    std::thread::sleep(Duration::from_secs(1));

    // Verify B is now stopped
    let output = run_cli(&["list"]);
    let service_b_line = output.lines().find(|line| line.contains("service-b"));
    if let Some(line) = service_b_line {
        assert!(
            !line.contains("running"),
            "service-b should be stopped. line: {}",
            line
        );
    }

    // Verify A is running
    assert!(
        assert_process_state_by_name("service-a", "running"),
        "service-a should be running"
    );

    // Daemon cleanup handled by guard
}

/// Test that Conflicts is shown in describe output
#[test]
fn test_conflicts_in_describe() {
    let _daemon = setup_daemon();

    // Create process with conflicts
    create_process(
        "test-service",
        "sleep",
        &["3600", "--conflicts", "other-service"],
    );

    // Describe the service
    let output = run_cli(&["describe", "test-service"]);
    assert!(
        output.contains("Conflicts"),
        "describe output should show Conflicts field. output: {}",
        output
    );
    assert!(
        output.contains("other-service"),
        "describe output should show the conflicting service. output: {}",
        output
    );

    // Daemon cleanup handled by guard
}

/// Test process without conflicts works normally
#[test]
fn test_no_conflicts_works_normally() {
    let _daemon = setup_daemon();

    // Create and start two independent services (no conflicts)
    let _id1 = create_and_start_process("service-x", "sleep", &["3600"]);
    let _id2 = create_and_start_process("service-y", "sleep", &["3600"]);

    std::thread::sleep(Duration::from_millis(300));

    // Verify both are running
    assert!(
        assert_process_state_by_name("service-x", "running"),
        "service-x should be running"
    );
    assert!(
        assert_process_state_by_name("service-y", "running"),
        "service-y should be running"
    );

    // Daemon cleanup handled by guard
}

/// Test systemd's bidirectional Conflicts behavior:
/// If A has Conflicts=B, then starting A stops B AND starting B stops A
/// This should work even when only ONE process declares the conflict
#[test]
fn test_unidirectional_conflicts_has_bidirectional_effect() {
    let _daemon = setup_daemon();

    let service_a = unique_process_name();
    let service_b = unique_process_name();

    // Create process B WITH Conflicts=service-a (only B declares the conflict)
    create_process(&service_b, "sleep", &["3600", "--conflicts", &service_a]);

    // === PART 1: Forward conflict (B has Conflicts=A, starting B stops A) ===

    // Create and start service-a first (no conflicts declared)
    let _id = create_and_start_process(&service_a, "sleep", &["3600"]);
    std::thread::sleep(Duration::from_millis(500));

    // Verify A is running
    let output = run_cli(&["list"]);
    assert!(
        output.contains(&service_a)
            && output
                .lines()
                .any(|line| line.contains(&service_a) && line.contains("running")),
        "service-a should be running"
    );

    // Start service-b → should STOP service-a (forward conflict)
    start_process(&service_b);
    std::thread::sleep(Duration::from_millis(500));

    // Verify B is running and A is NOT running
    let output = run_cli(&["list"]);
    assert!(
        output
            .lines()
            .any(|line| line.contains(&service_b) && line.contains("running")),
        "service-b should be running"
    );
    assert!(
        !output
            .lines()
            .any(|line| line.contains(&service_a) && line.contains("running")),
        "service-a should NOT be running (stopped by forward conflict)"
    );

    // === PART 2: Reverse conflict (starting A stops B, even though A doesn't declare conflict) ===

    // Now start service-a → should STOP service-b (reverse/bidirectional conflict)
    start_process(&service_a);
    std::thread::sleep(Duration::from_millis(500));

    // Verify A is running and B is NOT running
    let output = run_cli(&["list"]);
    assert!(
        output
            .lines()
            .any(|line| line.contains(&service_a) && line.contains("running")),
        "service-a should be running"
    );
    assert!(
        !output
            .lines()
            .any(|line| line.contains(&service_b) && line.contains("running")),
        "service-b should NOT be running (stopped by reverse/bidirectional conflict)"
    );

    // Daemon cleanup handled by guard
}
