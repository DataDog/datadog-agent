use std::thread;
use std::time::Duration;

use pm_e2e_tests::{
    create_process, delete_process, extract_process_id, run_cli, run_cli_full, setup_daemon,
    setup_daemon_with_config_file, start_process, stop_process, unique_process_name,
};

#[test]
fn test_e2e_duplicate_name_rejection() {
    let _daemon = setup_daemon();

    let process_name = unique_process_name();

    // Create first process
    let (stdout1, stderr1, exit_code1) = run_cli_full(&["create", &process_name, "sleep", "300"]);
    assert!(
        exit_code1 == 0,
        "First process should be created. stdout: {}, stderr: {}",
        stdout1,
        stderr1
    );

    // Try to create second process with same name
    let (stdout2, stderr2, exit_code2) = run_cli_full(&["create", &process_name, "echo", "hello"]);
    assert!(
        exit_code2 != 0,
        "Second process creation should fail due to duplicate name"
    );

    // Check error message contains "already exists"
    let error_msg = format!("{}{}", stdout2, stderr2);
    assert!(
        error_msg.contains("already exists"),
        "Error message should mention 'already exists'. Got: {}",
        error_msg
    );

    // Verify only one process exists
    let list_output = run_cli(&["list"]);
    let process_name_count = list_output.matches(&process_name).count();
    assert_eq!(
        process_name_count, 1,
        "Should only have one process named '{}'",
        process_name
    );

    // Clean up
    delete_process(&process_name);

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_recreate_after_delete() {
    let _daemon = setup_daemon();

    thread::sleep(Duration::from_millis(500));

    // Create first process
    create_process("webapp", "sleep", &["100"]);

    // Delete it
    delete_process("webapp");

    // Should be able to create again with same name
    create_process("webapp", "sleep", &["200"]);

    // Verify it exists
    let describe = run_cli(&["describe", "webapp"]);
    assert!(describe.contains("webapp"), "Process should be recreated");
    assert!(
        describe.contains("sleep"),
        "Process should have sleep command"
    );

    // Clean up
    delete_process("webapp");

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_multiple_unique_names() {
    let _daemon = setup_daemon();

    thread::sleep(Duration::from_millis(500));

    // Create multiple processes with unique names
    let names = vec!["app1", "app2", "app3", "worker-1", "worker-2"];

    for name in &names {
        create_process(name, "sleep", &["300"]);
    }

    // Verify all exist
    let list_output = run_cli(&["list"]);
    for name in &names {
        assert!(list_output.contains(name), "List should contain {}", name);
    }

    // Clean up
    for name in &names {
        delete_process(name);
    }

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_operations_by_name_and_uuid() {
    let _daemon = setup_daemon();

    thread::sleep(Duration::from_millis(500));

    // Create process
    let output = run_cli(&["create", "test-proc", "sleep", "300"]);
    let id = extract_process_id(&output).expect("Should extract ID");

    // Start by name
    start_process("test-proc");
    thread::sleep(Duration::from_millis(200));

    // Describe by UUID
    let describe_by_id = run_cli(&["describe", id]);
    assert!(describe_by_id.contains("test-proc"));
    assert!(describe_by_id.contains("running"));

    // Stop by UUID
    stop_process(id);
    thread::sleep(Duration::from_millis(200));

    // Describe by name
    let describe_by_name = run_cli(&["describe", "test-proc"]);
    assert!(describe_by_name.contains("stopped"));

    // Delete by name
    delete_process("test-proc");

    // Verify it's gone (both name and UUID should fail)
    let list_output = run_cli(&["list"]);
    assert!(!list_output.contains("test-proc"));

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_yaml_loaded_process_prevents_duplicate_cli_create() {
    // Create YAML config with a process
    let yaml_content = r#"processes:
  yaml-app:
    command: sleep
    args: ["500"]
"#;
    std::fs::write("/tmp/test-yaml-unique.yaml", yaml_content).unwrap();

    // Start daemon WITH config (this loads yaml-app)
    let _daemon = setup_daemon_with_config_file("/tmp/test-yaml-unique.yaml");
    thread::sleep(Duration::from_secs(2));

    // Verify YAML process was loaded
    let list_output = run_cli(&["list"]);
    assert!(
        list_output.contains("yaml-app"),
        "Process from YAML should be loaded. Output: {}",
        list_output
    );

    // Try to create duplicate via CLI - should fail
    let (stdout, stderr, exit_code) = run_cli_full(&["create", "yaml-app", "echo", "test"]);
    assert!(
        exit_code != 0,
        "CLI create should fail for name already loaded from YAML"
    );

    let error_msg = format!("{}{}", stdout, stderr);
    assert!(
        error_msg.contains("already exists"),
        "Error should mention 'already exists'. Got: {}",
        error_msg
    );

    // Verify original YAML process is still there and unchanged
    let describe = run_cli(&["describe", "yaml-app"]);
    assert!(
        describe.contains("sleep"),
        "YAML process should be unchanged"
    );
    assert!(
        describe.contains("500"),
        "YAML process args should be unchanged"
    );

    // Clean up
    delete_process("yaml-app");
    std::fs::remove_file("/tmp/test-yaml-unique.yaml").ok();

    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_yaml_with_duplicate_keys_rejected() {
    // Use unique filename to avoid conflicts with other tests
    let yaml_path = format!("/tmp/test-dup-keys-{}.yaml", std::process::id());

    // Create YAML config with duplicate keys (invalid YAML)
    let yaml_content = r#"processes:
  app:
    command: sleep
    args: ["100"]
  app:
    command: echo
    args: ["hello"]
"#;
    std::fs::write(&yaml_path, yaml_content).unwrap();

    // Start daemon WITH duplicate config - should log error but start successfully
    let _daemon = setup_daemon_with_config_file(&yaml_path);
    thread::sleep(Duration::from_millis(1000)); // Longer wait for CI

    // Daemon should have started but NO processes should be loaded
    // The duplicate key error should prevent the config from loading
    let list_output = run_cli(&["list"]);

    // More specific check: should NOT have "app" process from the bad config
    // (may have other processes from create_process below, but not "app")
    let lines: Vec<&str> = list_output.lines().collect();
    let has_app_process = lines.iter().any(|line| {
        // Check if any line starts with "app " (the process name column)
        line.trim_start().starts_with("app ")
    });

    assert!(
        !has_app_process,
        "Should NOT have 'app' process from bad config. List output:\n{}",
        list_output
    );

    // Verify we can still create processes via CLI (daemon is functional)
    create_process("test-dup-process", "sleep", &["200"]);
    let list_output2 = run_cli(&["list"]);
    assert!(
        list_output2.contains("test-dup-process"),
        "Should be able to create processes via CLI"
    );

    // Clean up
    delete_process("test-dup-process");
    std::fs::remove_file(&yaml_path).ok();

    // Daemon cleanup handled by guard
}
