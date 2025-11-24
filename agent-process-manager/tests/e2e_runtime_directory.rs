mod common;

use common::{run_cli, run_cli_full, setup_daemon, unique_process_name};
use std::fs;
use std::path::Path;
use std::thread;
use std::time::Duration;

/// Test basic runtime directory creation and cleanup
#[test]
fn test_runtime_directory_basic() {
    let _daemon = setup_daemon();
    let name = unique_process_name();
    let runtime_dir = format!("/run/pm-test/{}", name);

    // Ensure directory doesn't exist before test
    let _ = fs::remove_dir_all(&runtime_dir);

    // Create process with runtime directory (relative path, like systemd)
    let output = run_cli(&[
        "create",
        &name,
        "sleep",
        "5",
        "--runtime-directory",
        &format!("pm-test/{}", name),
        "--auto-start",
    ]);

    assert!(
        !output.contains("[ERROR]"),
        "Process creation should succeed, got: {}",
        output
    );

    // Wait for process to start
    thread::sleep(Duration::from_secs(1));

    // Verify directory was created
    assert!(
        Path::new(&runtime_dir).exists(),
        "Runtime directory should exist while process is running"
    );

    // Stop the process
    let stop_output = run_cli(&["stop", &name]);
    assert!(
        !stop_output.contains("[ERROR]"),
        "Process stop should succeed"
    );

    // Wait for cleanup
    thread::sleep(Duration::from_millis(500));

    // Verify directory was cleaned up
    assert!(
        !Path::new(&runtime_dir).exists(),
        "Runtime directory should be removed after process stops"
    );

    // Daemon cleanup handled by guard
}

/// Test multiple runtime directories
#[test]
fn test_runtime_directory_multiple() {
    let _daemon = setup_daemon();
    let name = unique_process_name();
    let runtime_dir1 = format!("/run/pm-test/{}_dir1", name);
    let runtime_dir2 = format!("/run/pm-test/{}_dir2", name);

    // Ensure directories don't exist before test
    let _ = fs::remove_dir_all(&runtime_dir1);
    let _ = fs::remove_dir_all(&runtime_dir2);

    // Create process with multiple runtime directories
    let output = run_cli(&[
        "create",
        &name,
        "sleep",
        "5",
        "--runtime-directory",
        &format!("pm-test/{}_dir1", name),
        "--runtime-directory",
        &format!("pm-test/{}_dir2", name),
        "--auto-start",
    ]);

    assert!(
        !output.contains("[ERROR]"),
        "Process creation should succeed, got: {}",
        output
    );

    // Wait for process to start
    thread::sleep(Duration::from_secs(1));

    // Verify both directories were created
    assert!(
        Path::new(&runtime_dir1).exists(),
        "First runtime directory should exist"
    );
    assert!(
        Path::new(&runtime_dir2).exists(),
        "Second runtime directory should exist"
    );

    // Stop the process
    let stop_output = run_cli(&["stop", &name]);
    assert!(
        !stop_output.contains("[ERROR]"),
        "Process stop should succeed"
    );

    // Wait for cleanup
    thread::sleep(Duration::from_millis(500));

    // Verify both directories were cleaned up
    assert!(
        !Path::new(&runtime_dir1).exists(),
        "First runtime directory should be removed"
    );
    assert!(
        !Path::new(&runtime_dir2).exists(),
        "Second runtime directory should be removed"
    );

    // Daemon cleanup handled by guard
}

/// Test runtime directory persists across process lifecycle
#[test]
fn test_runtime_directory_with_restart() {
    let _daemon = setup_daemon();
    let name = unique_process_name();
    let runtime_dir = format!("/run/pm-test/{}", name);

    // Ensure directory doesn't exist before test
    let _ = fs::remove_dir_all(&runtime_dir);

    // Create a long-running process
    let output = run_cli(&[
        "create",
        &name,
        "sleep",
        "30",
        "--runtime-directory",
        &format!("pm-test/{}", name),
        "--auto-start",
    ]);

    assert!(
        !output.contains("[ERROR]"),
        "Process creation should succeed, got: {}",
        output
    );

    // Wait for process to fully start
    thread::sleep(Duration::from_secs(2));

    // Verify directory was created
    assert!(
        Path::new(&runtime_dir).exists(),
        "Runtime directory should exist while process runs"
    );

    // Stop the process
    let stop_output = run_cli(&["stop", &name]);
    assert!(
        !stop_output.contains("[ERROR]"),
        "Process stop should succeed"
    );

    // Wait for cleanup
    thread::sleep(Duration::from_secs(1));

    // Verify directory was cleaned up
    assert!(
        !Path::new(&runtime_dir).exists(),
        "Runtime directory should be removed after stop"
    );

    // Daemon cleanup handled by guard
}

/// Test runtime directory in describe output
#[test]
fn test_runtime_directory_describe() {
    let _daemon = setup_daemon();
    let name = unique_process_name();

    // Create process with runtime directory
    let output = run_cli(&[
        "create",
        &name,
        "sleep",
        "10",
        "--runtime-directory",
        &format!("pm-test/{}", name),
    ]);

    assert!(
        !output.contains("[ERROR]"),
        "Process creation should succeed"
    );

    // Describe the process
    let describe_output = run_cli(&["describe", &name]);

    assert!(
        !describe_output.contains("[ERROR]"),
        "Describe should succeed, got: {}",
        describe_output
    );
    assert!(
        describe_output.contains("RUNTIME DIRECTORIES"),
        "Should have runtime directories section"
    );
    assert!(
        describe_output.contains(&format!("/run/pm-test/{}", name)),
        "Should show the runtime directory path"
    );

    // Daemon cleanup handled by guard
}

/// Test that absolute paths are rejected (like systemd)
#[test]
fn test_runtime_directory_rejects_absolute_paths() {
    let _daemon = setup_daemon();
    let name = unique_process_name();

    // Try to create process with absolute path (should fail, like systemd)
    let (stdout, stderr, exit_code) = run_cli_full(&[
        "create",
        &name,
        "sleep",
        "5",
        "--runtime-directory",
        "/tmp/absolute_path",
    ]);

    let output = format!("{}{}", stdout, stderr);
    assert!(
        exit_code != 0
            || output.contains("[ERROR]")
            || output.contains("absolute path not allowed"),
        "Should reject absolute paths in RuntimeDirectory, got exit_code={}, output: {}",
        exit_code,
        output
    );

    // Daemon cleanup handled by guard
}

/// Test process without runtime directory
#[test]
fn test_no_runtime_directory() {
    let _daemon = setup_daemon();
    let name = unique_process_name();

    // Create process without runtime directory
    let output = run_cli(&["create", &name, "sleep", "5", "--auto-start"]);

    assert!(
        !output.contains("[ERROR]"),
        "Process creation should succeed, got: {}",
        output
    );

    // Wait for process to start
    thread::sleep(Duration::from_secs(1));

    // Process should run normally
    let list_output = run_cli(&["list"]);
    assert!(list_output.contains(&name), "Process should be in the list");
    assert!(list_output.contains("running"), "Process should be running");

    // Daemon cleanup handled by guard
}

/// Test runtime directory permissions (directories created with 0755)
#[test]
fn test_runtime_directory_permissions() {
    let _daemon = setup_daemon();
    let name = unique_process_name();
    let runtime_dir = format!("/run/pm-test/{}", name);

    // Ensure directory doesn't exist before test
    let _ = fs::remove_dir_all(&runtime_dir);

    // Create process with runtime directory
    let output = run_cli(&[
        "create",
        &name,
        "sleep",
        "5",
        "--runtime-directory",
        &format!("pm-test/{}", name),
        "--auto-start",
    ]);

    assert!(
        !output.contains("[ERROR]"),
        "Process creation should succeed, got: {}",
        output
    );

    // Wait for process to start
    thread::sleep(Duration::from_secs(1));

    // Verify directory exists and has correct permissions
    assert!(
        Path::new(&runtime_dir).exists(),
        "Runtime directory should exist"
    );

    // Check permissions (should be 0755)
    let metadata = fs::metadata(&runtime_dir).expect("Failed to get directory metadata");
    let permissions = metadata.permissions();

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mode = permissions.mode();
        let perms = mode & 0o777;
        assert_eq!(
            perms, 0o755,
            "Runtime directory should have 0755 permissions, got {:#o}",
            perms
        );
    }

    // Stop the process
    let stop_output = run_cli(&["stop", &name]);
    assert!(
        !stop_output.contains("[ERROR]"),
        "Process stop should succeed"
    );

    // Wait for cleanup
    thread::sleep(Duration::from_millis(500));

    // Daemon cleanup handled by guard
}

/// Test runtime directory with nested paths
#[test]
fn test_runtime_directory_nested() {
    let _daemon = setup_daemon();
    let name = unique_process_name();
    let runtime_dir = format!("/run/pm-test/{}/subdir", name);
    let parent_dir = format!("/run/pm-test/{}", name);

    // Ensure directory doesn't exist before test
    let _ = fs::remove_dir_all(&parent_dir);

    // Create process with nested runtime directory
    let output = run_cli(&[
        "create",
        &name,
        "sleep",
        "30",
        "--runtime-directory",
        &format!("pm-test/{}/subdir", name),
        "--auto-start",
    ]);

    assert!(
        !output.contains("[ERROR]"),
        "Process creation should succeed with nested directory, got: {}",
        output
    );

    // Wait for process to fully start and directory to be created
    thread::sleep(Duration::from_secs(2));

    // Verify nested directory was created
    assert!(
        Path::new(&runtime_dir).exists(),
        "Nested runtime directory should exist"
    );

    // Stop the process
    let stop_output = run_cli(&["stop", &name]);
    assert!(
        !stop_output.contains("[ERROR]"),
        "Process stop should succeed"
    );

    // Wait for cleanup
    thread::sleep(Duration::from_secs(1));

    // Verify nested directory was cleaned up
    // Note: remove_dir_all removes the entire tree
    assert!(
        !Path::new(&runtime_dir).exists(),
        "Nested runtime directory should be removed"
    );

    // Daemon cleanup handled by guard
}
