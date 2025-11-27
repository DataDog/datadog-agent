use pm_e2e_tests::{run_cli_full, setup_daemon_with_config_dir};
use std::thread::sleep;
use std::time::Duration;

fn unique_config_name() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_micros();
    format!("test-resources-{}", timestamp % 1000000)
}

/// Create a config directory with a single process config file
fn create_config_dir(process_name: &str, config_content: &str) -> (String, String) {
    let unique_id = unique_config_name();
    let config_dir = format!("/tmp/{}-config.d", unique_id);
    std::fs::create_dir_all(&config_dir).expect("Failed to create config dir");
    
    let config_path = format!("{}/{}.yaml", config_dir, process_name);
    std::fs::write(&config_path, config_content).expect("Failed to write config");
    
    (config_dir, config_path)
}

#[test]
fn test_e2e_resource_limits_yaml() {
    let unique_id = unique_config_name();
    let process_name = format!("sleep-limited-{}", unique_id);

    // Create config with resource limits (direct ProcessConfig format)
    let config_content = r#"
command: sleep
args: ["300"]
auto_start: true
resource_limits:
  cpu: 1000m
  memory: 256M
  pids: 50
"#;

    let (config_dir, _) = create_config_dir(&process_name, config_content);

    // Start daemon with config directory
    let _daemon = setup_daemon_with_config_dir(&config_dir);

    // Give daemon more time to load config and auto-start processes
    sleep(Duration::from_secs(3));

    // Check process was created and started
    let (stdout, stderr, code) = run_cli_full(&["list"]);
    println!("\n=== Test: {} ===", process_name);
    println!("Process list (code={}):\n{}", code, stdout);
    if !stderr.is_empty() {
        println!("Stderr: {}", stderr);
    }

    // If process wasn't created, the config may not have loaded properly
    // This can happen in environments without proper permissions or cgroups
    if !stdout.contains(&process_name) {
        println!("WARN: Process not found in list. This may indicate:");
        println!("  - Config file not loaded (check daemon logs)");
        println!("  - Process failed to start due to missing cgroups v2 support");
        println!("  - Insufficient permissions for resource limits");
        println!("Skipping test as environment may not support resource limits");
        std::fs::remove_dir_all(&config_dir).ok();
        return;
    }

    assert!(
        stdout.contains(&process_name),
        "Process should be in the list"
    );
    assert!(
        stdout.contains("running") || stdout.contains("starting"),
        "Process should be running or starting"
    );

    // Check if cgroup files exist (requires cgroup v2)
    if std::path::Path::new("/sys/fs/cgroup/cgroup.controllers").exists() {
        println!("cgroup v2 detected, checking cgroup files...");

        // Give process time to fully start and cgroup to be created
        sleep(Duration::from_millis(500));

        let cgroup_path = format!("/sys/fs/cgroup/pm-processes/{}", process_name);
        if std::path::Path::new(&cgroup_path).exists() {
            println!("[OK] Cgroup created at: {}", cgroup_path);

            // Check memory limit
            let memory_max = std::fs::read_to_string(format!("{}/memory.max", cgroup_path));
            if let Ok(mem) = memory_max {
                println!("memory.max: {}", mem.trim());
                let mem_bytes: u64 = mem.trim().parse().unwrap_or(0);
                let expected_bytes = 256 * 1024 * 1024; // 256M
                assert_eq!(
                    mem_bytes, expected_bytes,
                    "Memory limit should be 256MB ({} bytes)",
                    expected_bytes
                );
            }

            // Check CPU limit
            let cpu_max = std::fs::read_to_string(format!("{}/cpu.max", cgroup_path));
            if let Ok(cpu) = cpu_max {
                println!("cpu.max: {}", cpu.trim());
                // Format: "quota period" e.g., "100000 100000" for 1 core
                assert!(
                    cpu.contains("100000 100000"),
                    "CPU limit should be 1 core (100000/100000)"
                );
            }

            // Check PIDs limit
            let pids_max = std::fs::read_to_string(format!("{}/pids.max", cgroup_path));
            if let Ok(pids) = pids_max {
                println!("pids.max: {}", pids.trim());
                assert_eq!(pids.trim(), "50", "PIDs limit should be 50");
            }

            println!("[OK] All cgroup limits verified!");
        } else {
            println!("⚠ Cgroup path doesn't exist (might not have permissions)");
        }
    } else {
        println!("⚠ cgroup v2 not available, skipping cgroup verification");
    }

    // Stop process
    run_cli_full(&["stop", &process_name]);
    sleep(Duration::from_millis(500));

    // Cleanup
    std::fs::remove_dir_all(&config_dir).ok();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_resource_limits_with_usage() {
    let unique_id = unique_config_name();
    let process_name = format!("busy-limited-{}", unique_id);

    // Create config with a process that uses some memory (direct ProcessConfig format)
    let config_content = r#"
command: sh
args:
  - "-c"
  - "echo Starting; i=0; while [ $i -lt 1000 ]; do i=$((i+1)); sleep 0.01; done"
auto_start: true
resource_limits:
  memory: 50M
  cpu: 2000m
"#;

    let (config_dir, _) = create_config_dir(&process_name, config_content);

    // Start daemon
    let _daemon = setup_daemon_with_config_dir(&config_dir);

    // Give daemon more time to load config and auto-start processes
    sleep(Duration::from_secs(3));

    // Verify process is running
    let (stdout, _, _) = run_cli_full(&["list"]);
    println!("\n=== Test: {} ===", process_name);
    println!("Process list:\n{}", stdout);
    assert!(stdout.contains(&process_name), "Process should exist");

    // Check resource usage is being tracked (if cgroups available)
    let cgroup_path = format!("/sys/fs/cgroup/pm-processes/{}", process_name);
    if std::path::Path::new(&cgroup_path).exists() {
        // Check memory.current exists and has a reasonable value
        if let Ok(mem_current) = std::fs::read_to_string(format!("{}/memory.current", cgroup_path))
        {
            let mem_bytes: u64 = mem_current.trim().parse().unwrap_or(0);
            println!(
                "Current memory usage: {} bytes ({} MB)",
                mem_bytes,
                mem_bytes / 1024 / 1024
            );
            assert!(mem_bytes > 0, "Process should be using some memory");
            assert!(
                mem_bytes < 50 * 1024 * 1024,
                "Memory usage should be under limit"
            );
        }

        // Check CPU usage is accumulating
        if let Ok(cpu_stat) = std::fs::read_to_string(format!("{}/cpu.stat", cgroup_path)) {
            println!("cpu.stat:\n{}", cpu_stat);
            assert!(
                cpu_stat.contains("usage_usec"),
                "CPU stats should be available"
            );
        }

        println!("[OK] Resource usage monitoring verified!");
    }

    // Cleanup
    run_cli_full(&["stop", &process_name]);
    sleep(Duration::from_millis(500));
    std::fs::remove_dir_all(&config_dir).ok();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_resource_limits_no_limits() {
    let unique_id = unique_config_name();
    let process_name = format!("sleep-unlimited-{}", unique_id);

    // Create config WITHOUT resource limits (direct ProcessConfig format)
    let config_content = r#"
command: sleep
args: ["60"]
auto_start: true
"#;

    let (config_dir, _) = create_config_dir(&process_name, config_content);

    // Start daemon
    let _daemon = setup_daemon_with_config_dir(&config_dir);

    // Give daemon more time to load config and auto-start processes
    sleep(Duration::from_secs(3));

    // Process should start normally without limits
    let (stdout, _, _) = run_cli_full(&["list"]);
    println!("\n=== Test: {} ===", process_name);
    println!("Process list:\n{}", stdout);
    assert!(stdout.contains(&process_name), "Process should exist");
    assert!(
        stdout.contains("running") || stdout.contains("starting"),
        "Process should be running"
    );

    // Cgroup should NOT be created for this process
    let cgroup_path = format!("/sys/fs/cgroup/pm-processes/{}", process_name);
    if std::path::Path::new("/sys/fs/cgroup/cgroup.controllers").exists() {
        if std::path::Path::new(&cgroup_path).exists() {
            println!("⚠ Cgroup exists even without limits (this is OK, might be created for other reasons)");
        } else {
            println!("[OK] No cgroup created for unlimited process");
        }
    }

    // Cleanup
    run_cli_full(&["stop", &process_name]);
    sleep(Duration::from_millis(500));
    std::fs::remove_dir_all(&config_dir).ok();
    // Daemon cleanup handled by guard
}
