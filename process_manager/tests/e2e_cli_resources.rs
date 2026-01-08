mod common;

use common::{create_process, delete_process, run_cli_full, setup_daemon, stop_process};
use std::thread::sleep;
use std::time::Duration;

#[test]
fn test_e2e_cli_resource_flags() {
    // Start daemon
    let _daemon = setup_daemon();
    sleep(Duration::from_secs(1));

    let process_name = "resource-test-cli";

    // Create process with resource limits and auto-start via CLI flags
    let _id = create_process(
        process_name,
        "sleep",
        &[
            "3600",
            "--cpu-limit",
            "1000m",
            "--memory-limit",
            "256M",
            "--pids-limit",
            "100",
            "--auto-start",
        ],
    );

    // Give process time to fully start
    sleep(Duration::from_secs(1));

    // Verify resource limits are set via describe
    let (stdout, _, _) = run_cli_full(&["describe", process_name]);
    println!("\nDescribe output:\n{}", stdout);

    // Check that resource limits section exists
    assert!(
        stdout.contains("RESOURCE LIMITS"),
        "Should show resource limits section"
    );
    assert!(
        stdout.contains("CPU Limit:       1000m"),
        "Should show CPU limit"
    );
    assert!(
        stdout.contains("Memory Limit:    256 MB"),
        "Should show memory limit"
    );
    assert!(
        stdout.contains("PIDs Limit:      100"),
        "Should show PIDs limit"
    );

    // Test stats command (may not work if cgroups unavailable, but should not error)
    let (stdout, _, _code) = run_cli_full(&["stats", process_name]);
    println!("\nStats output:\n{}", stdout);

    // Check if stats worked or gracefully failed
    if stdout.contains("Resource Usage Statistics") {
        // If stats works, verify it has the expected sections
        println!("[OK] Stats command succeeded");
    } else if stdout.contains("[ERROR]") && stdout.contains("Resource usage not available") {
        // If cgroups unavailable, that's okay for this test - the important part is that
        // the CLI handled it gracefully and the limits were properly set
        println!("[OK] Stats command gracefully handled unavailable cgroups");
    } else {
        panic!("Stats command produced unexpected output: {}", stdout);
    }

    // Cleanup
    stop_process(process_name);
    delete_process(process_name);
    // Daemon cleanup handled by guard
}
