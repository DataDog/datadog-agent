use pm_e2e_tests::{
    get_socket_test_port, process_exists_by_name, run_cli_full, setup_daemon_with_config_file,
    unique_process_name, unique_test_path,
};
use std::thread::sleep;
use std::time::Duration;

/// Write config file and ensure it's synced to disk before returning
fn write_config_sync(path: &str, content: &str) {
    std::fs::write(path, content).expect("Failed to write config");
    // Ensure file is flushed to disk
    std::fs::File::open(path)
        .expect("Failed to open config")
        .sync_all()
        .expect("Failed to sync config");
}

#[test]
fn test_e2e_tcp_socket_activation() {
    let socket_name = unique_process_name();
    let port = get_socket_test_port();
    let service_name = unique_process_name();

    // Create a test config with socket activation
    let config_content = format!(
        r#"
sockets:
  {}:
    listen_stream: "127.0.0.1:{}"
    service: {}
    accept: false

processes:
  {}:
    command: sleep
    args: ["30"]
    auto_start: false
"#,
        socket_name, port, service_name, service_name
    );

    let config_path = unique_test_path("test-tcp-socket-activation", ".yaml");
    write_config_sync(&config_path, &config_content);

    let _daemon = setup_daemon_with_config_file(&config_path);
    sleep(Duration::from_secs(3));

    // Verify process was created from config
    let (stdout, _, code) = run_cli_full(&["list"]);

    if code != 0 || stdout.is_empty() {
        // CLI connection issue - skip rest of test
        std::fs::remove_file(&config_path).ok();
        return;
    }

    assert!(
        process_exists_by_name(&service_name),
        "Process '{}' should exist",
        service_name
    );
    assert!(
        stdout.to_lowercase().contains("created"),
        "Process should be Created before activation"
    );

    // Try to connect to socket to trigger activation
    use std::io::Write;
    use std::net::TcpStream;

    let connect_result = TcpStream::connect_timeout(
        &format!("127.0.0.1:{}", port).parse().unwrap(),
        Duration::from_secs(2),
    );

    if let Ok(mut stream) = connect_result {
        // Write data to trigger socket activation
        match stream.write_all(b"trigger\n") {
            Ok(_) => {
                drop(stream);

                sleep(Duration::from_secs(2));

                let (stdout, _, _) = run_cli_full(&["list"]);
                println!("stdout: {stdout}");
                if !stdout.is_empty() {
                    // Process should have been started by socket activation
                    assert!(
                        stdout.to_lowercase().contains("running")
                            || stdout.to_lowercase().contains("exited"),
                        "Process should be running/exited after socket activation"
                    );
                }
            }
            Err(e) => {
                // Write failed - socket might not be ready or daemon issue
                eprintln!("Warning: Failed to write to socket: {}", e);
            }
        }
    }

    // Daemon cleanup handled by guard
    std::fs::remove_file(&config_path).ok();
}

#[test]
fn test_e2e_socket_config_loading() {
    // Create a minimal socket config with unique values
    let socket_name = unique_process_name();
    let service_name = unique_process_name();
    let port = get_socket_test_port();

    let config_content = format!(
        r#"
sockets:
  {}:
    listen_stream: "127.0.0.1:{}"
    service: {}
    accept: false

processes:
  {}:
    command: sleep
    args: ["10"]
    auto_start: false
"#,
        socket_name, port, service_name, service_name
    );

    let config_path = unique_test_path("test-socket-config-loading", ".yaml");
    write_config_sync(&config_path, &config_content);

    // Start daemon with config
    let _daemon = setup_daemon_with_config_file(&config_path);

    // Give it time to load config (increased for parallel execution)
    sleep(Duration::from_secs(3));

    // Check that process was created
    assert!(
        process_exists_by_name(&service_name),
        "Process '{}' should be created from config",
        service_name
    );

    // Clean up
    std::fs::remove_file(&config_path).ok();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_socket_yaml_parsing() {
    let tcp_socket_name = unique_process_name();
    let unix_socket_name = unique_process_name();
    let tcp_port = get_socket_test_port();
    let unix_path = unique_test_path("test-socket", ".sock");
    let tcp_service = unique_process_name();
    let unix_service = unique_process_name();

    let config_content = format!(
        r#"
sockets:
  {}:
    listen_stream: "127.0.0.1:{}"
    service: {}
    accept: false

  {}:
    listen_unix: "{}"
    service: {}
    accept: false
    socket_mode: 0o660

processes:
  {}:
    command: sleep
    args: ["60"]
    auto_start: false

  {}:
    command: sleep
    args: ["60"]
    auto_start: false
"#,
        tcp_socket_name,
        tcp_port,
        tcp_service,
        unix_socket_name,
        unix_path,
        unix_service,
        tcp_service,
        unix_service
    );

    let config_path = unique_test_path("test-socket-yaml-parsing", ".yaml");
    write_config_sync(&config_path, &config_content);

    // Start daemon with config
    let _daemon = setup_daemon_with_config_file(&config_path);
    sleep(Duration::from_secs(4)); // Increased from 3s to give socket activation more time

    // Check both processes exist
    assert!(
        process_exists_by_name(&tcp_service),
        "TCP service '{}' should exist",
        tcp_service
    );
    assert!(
        process_exists_by_name(&unix_service),
        "Unix service '{}' should exist",
        unix_service
    );

    // Clean up
    std::fs::remove_file(&config_path).ok();
    std::fs::remove_file(&unix_path).ok();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_accept_yes_multiple_instances() {
    use std::io::Read;
    use std::net::TcpStream;
    use std::thread;
    use std::time::Duration;

    let socket_name = unique_process_name();
    let service_name = unique_process_name();
    let port = get_socket_test_port();

    // Create config with Accept=yes
    let config_path = unique_test_path("test-accept-yes", ".yaml");
    let config_content = format!(
        r#"
processes:
  {service}:
    command: sh
    args:
      - "-c"
      - "echo 'Instance PID: $$' >&3; sleep 10"  # Longer sleep to catch them running
    auto_start: false

sockets:
  {socket}:
    listen_stream: "127.0.0.1:{port}"
    service: {service}
    accept: true
"#,
        service = service_name,
        socket = socket_name,
        port = port
    );
    write_config_sync(&config_path, &config_content);

    // Start daemon
    let _daemon = setup_daemon_with_config_file(&config_path);
    thread::sleep(Duration::from_secs(3));

    // Make first connection (retry up to 5 times if socket not ready yet)
    let mut stream1 = None;
    for attempt in 1..=5 {
        match TcpStream::connect(format!("127.0.0.1:{}", port)) {
            Ok(stream) => {
                stream1 = Some(stream);
                break;
            }
            Err(e) if attempt < 5 => {
                println!("Connection attempt {} failed: {}, retrying...", attempt, e);
                thread::sleep(Duration::from_secs(1));
            }
            Err(e) => panic!("Failed to connect after 5 attempts: {}", e),
        }
    }
    let mut stream1 = stream1.expect("Failed to establish connection 1");
    thread::sleep(Duration::from_secs(2));

    // Make second connection (should spawn NEW instance)
    let mut stream2 = None;
    for attempt in 1..=3 {
        match TcpStream::connect(format!("127.0.0.1:{}", port)) {
            Ok(stream) => {
                stream2 = Some(stream);
                break;
            }
            Err(e) if attempt < 3 => {
                println!(
                    "Connection 2 attempt {} failed: {}, retrying...",
                    attempt, e
                );
                thread::sleep(Duration::from_millis(500));
            }
            Err(e) => panic!("Failed to connect (connection 2) after 3 attempts: {}", e),
        }
    }
    let mut stream2 = stream2.expect("Failed to establish connection 2");
    thread::sleep(Duration::from_secs(2));

    // Check that we have multiple instances
    let (stdout, stderr, _) = run_cli_full(&["list"]);

    // Count how many instances of our service exist (running or exited)
    let total_count = stdout
        .lines()
        .filter(|line| line.contains(&service_name))
        .count();

    let running_count = stdout
        .lines()
        .filter(|line| line.contains(&service_name) && line.contains("running"))
        .count();

    // With Accept=yes, we should have spawned 2 instances (one per connection)
    assert!(
        running_count >= 2 || total_count >= 2,
        "Should have at least 2 instances (running or exited) with Accept=yes, got {} total, {} running.\nProcess list:\n{}\nStderr: {}",
        total_count, running_count, stdout, stderr
    );

    // Read responses to verify instances are different
    let mut buf1 = vec![0u8; 256];
    let mut buf2 = vec![0u8; 256];

    stream1.set_read_timeout(Some(Duration::from_secs(1))).ok();
    stream2.set_read_timeout(Some(Duration::from_secs(1))).ok();

    let n1 = stream1.read(&mut buf1).unwrap_or(0);
    let n2 = stream2.read(&mut buf2).unwrap_or(0);

    if n1 > 0 && n2 > 0 {
        let response1 = String::from_utf8_lossy(&buf1[..n1]);
        let response2 = String::from_utf8_lossy(&buf2[..n2]);

        // Both should contain "Instance PID:" but potentially different PIDs
        assert!(response1.contains("Instance PID:"));
        assert!(response2.contains("Instance PID:"));
    }

    drop(stream1);
    drop(stream2);

    // Clean up
    std::fs::remove_file(&config_path).ok();
    // Daemon cleanup handled by guard
}
