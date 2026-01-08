mod common;

use std::fs;
use std::io::{Read, Write};
use std::net::TcpStream;
use std::os::unix::net::UnixStream;
use std::time::Duration;

/// Create a config directory with process and socket files
fn create_socket_config_dir(
    service_name: &str,
    socket_name: &str,
    process_config: &str,
    socket_config: &str,
) -> String {
    let config_dir = format!("/tmp/socket-react-{}.d", std::process::id());
    fs::create_dir_all(&config_dir).expect("Failed to create config dir");

    // Write process config
    let process_path = format!("{}/{}.yaml", config_dir, service_name);
    fs::write(&process_path, process_config).expect("Failed to write process config");

    // Write socket config
    let socket_path = format!("{}/{}.socket.yaml", config_dir, socket_name);
    fs::write(&socket_path, socket_config).expect("Failed to write socket config");

    config_dir
}

#[test]
fn test_socket_activation_tcp_reactivation_after_crash() {
    // Use unique port for this test
    let test_socket_port = common::get_socket_test_port() + 1000; // Offset to avoid daemon port conflicts

    // Create a simple echo server config with socket activation
    let script_path =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("socket_echo_server.py");

    let process_config = format!(
        r#"
command: /usr/bin/python3
args: ["{}"]
working_dir: /tmp
auto_start: false  # Socket activation starts this
process_type: simple
restart: never  # No restart policy, only socket activation
stdout: /tmp/echo-stdout.log
stderr: /tmp/echo-stderr.log
"#,
        script_path.display()
    );

    let socket_config = format!(
        r#"
listen_stream: "127.0.0.1:{}"
service: echo-server
accept: false  # Accept=no: pass listening socket to single instance
"#,
        test_socket_port
    );

    let config_dir = create_socket_config_dir("echo-server", "echo-tcp", &process_config, &socket_config);

    let _daemon = common::setup_daemon_with_config_dir(&config_dir);

    // Give daemon time to initialize sockets
    std::thread::sleep(Duration::from_secs(3));

    // Step 1: Verify echo-server is NOT running initially
    // Retry loop to handle timing issues in CI
    let mut echo_line = None;
    for attempt in 1..=5 {
        let (list_out, _, _) = common::run_cli_full(&["list"]);
        if attempt == 1 || attempt == 5 {
            println!("List output (attempt {}):\n{}", attempt, list_out);
        }
        if let Some(line) = list_out.lines().find(|l| l.contains("echo-server")) {
            echo_line = Some(line.to_string());
            break;
        }
        std::thread::sleep(Duration::from_secs(1));
    }

    assert!(
        echo_line.is_some(),
        "Echo server should exist in process list after daemon initialization"
    );

    let echo_line = echo_line.unwrap();
    assert!(
        echo_line.contains("created") && !echo_line.contains("running"),
        "Echo server should be in 'created' state (not yet started). Line: {}",
        echo_line
    );

    // Step 2: Trigger socket activation by connecting
    println!(
        "Triggering initial socket activation on port {}...",
        test_socket_port
    );
    let mut stream = TcpStream::connect(format!("127.0.0.1:{}", test_socket_port))
        .expect("Should connect to socket");
    stream
        .set_read_timeout(Some(Duration::from_secs(5)))
        .unwrap();

    // Write some data
    stream.write_all(b"Hello\n").unwrap();

    // Read echo back (cat echoes input)
    let mut buffer = [0u8; 1024];
    let n = stream.read(&mut buffer).unwrap();
    let response = String::from_utf8_lossy(&buffer[..n]);
    println!("Echo response: {}", response);
    assert!(response.contains("Hello"), "Should echo input");
    drop(stream);

    std::thread::sleep(Duration::from_millis(500));

    // Step 3: Verify echo-server was activated (may have exited after handling request)
    let (list_out, _, _) = common::run_cli_full(&["list"]);
    println!("After activation:\n{}", list_out);

    // In Accept=no mode with simple services, the process handles one request and exits
    // This is normal! Check that it ran at least once.
    let lines: Vec<&str> = list_out.lines().collect();
    let echo_line = lines.iter().find(|l| l.contains("echo-server")).unwrap();
    assert!(
        echo_line.contains("exited") || echo_line.contains("running"),
        "Echo server should have been activated. Line: {}",
        echo_line
    );

    // Record the run count for later comparison
    let parts: Vec<&str> = echo_line.split_whitespace().collect();
    let runs1: u32 = parts[4].parse().expect("Should parse run count");
    println!("First activation - runs: {}", runs1);
    assert!(runs1 >= 1, "Should have run at least once");

    // Step 4: Process has already exited after handling the request
    // This is the expected behavior for Accept=no + simple service
    println!("Process handled request and exited (as expected for Accept=no mode)");

    // Give PM time to detect process exit and reset socket state
    std::thread::sleep(Duration::from_secs(2));

    // Step 5: THE MAGIC - Trigger socket re-activation!
    println!(
        "Triggering socket RE-ACTIVATION on port {}...",
        test_socket_port
    );
    let mut stream = TcpStream::connect(format!("127.0.0.1:{}", test_socket_port))
        .expect("Should connect again for re-activation");
    stream
        .set_read_timeout(Some(Duration::from_secs(5)))
        .unwrap();

    stream.write_all(b"World\n").unwrap();

    let mut buffer = [0u8; 1024];
    let n = stream.read(&mut buffer).unwrap();
    let response = String::from_utf8_lossy(&buffer[..n]);
    println!("Re-activated echo response: {}", response);
    assert!(
        response.contains("World"),
        "Should echo input from new instance"
    );
    drop(stream);

    std::thread::sleep(Duration::from_millis(500));

    // Step 5: Verify socket re-activation worked - run count should increment
    let (list_out, _, _) = common::run_cli_full(&["list"]);
    println!("After re-activation:\n{}", list_out);

    let echo_line = list_out
        .lines()
        .find(|l| l.contains("echo-server"))
        .unwrap();
    let parts: Vec<&str> = echo_line.split_whitespace().collect();
    let runs2: u32 = parts[4].parse().expect("Should parse run count");
    println!("After re-activation - runs: {}", runs2);

    assert_eq!(
        runs2,
        runs1 + 1,
        "Socket re-activation should have triggered a NEW run. Expected {}, got {}",
        runs1 + 1,
        runs2
    );

    println!(
        "[OK] Socket re-activation SUCCESS! Process ran {} times total",
        runs2
    );

    // Cleanup
    // Daemon cleanup handled by guard
}

#[test]
fn test_socket_activation_unix_reactivation_after_crash() {
    let temp_dir = common::setup_temp_dir();
    let socket_path = temp_dir.path().join("echo.sock");

    // Create config with Unix socket activation
    let script_path =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("socket_echo_server_unix.py");

    let process_config = format!(
        r#"
command: /usr/bin/python3
args: ["{}"]
working_dir: /tmp
auto_start: false
process_type: simple
restart: never
stdout: /tmp/echo-unix-stdout.log
stderr: /tmp/echo-unix-stderr.log
"#,
        script_path.display()
    );

    let socket_config = format!(
        r#"
listen_unix: "{}"
service: echo-unix
accept: false
socket_mode: "666"
"#,
        socket_path.display()
    );

    let config_dir = create_socket_config_dir("echo-unix", "echo-unix-socket", &process_config, &socket_config);

    let _daemon = common::setup_daemon_with_config_dir(&config_dir);

    // Give daemon time to initialize sockets
    std::thread::sleep(Duration::from_secs(3));

    // Step 1: Trigger initial activation
    println!("Connecting to Unix socket: {}", socket_path.display());
    let mut stream = UnixStream::connect(&socket_path).expect("Should connect to Unix socket");
    stream
        .set_read_timeout(Some(Duration::from_secs(5)))
        .unwrap();

    stream.write_all(b"Unix Test\n").unwrap();

    let mut buffer = [0u8; 1024];
    let n = stream.read(&mut buffer).unwrap();
    let response = String::from_utf8_lossy(&buffer[..n]);
    println!("Unix socket response: {}", response);
    assert!(response.contains("Unix Test"));
    drop(stream);

    std::thread::sleep(Duration::from_millis(500));

    // Step 2: Get run count
    let (list_out, _, _) = common::run_cli_full(&["list"]);
    let echo_line = list_out.lines().find(|l| l.contains("echo-unix")).unwrap();
    let parts: Vec<&str> = echo_line.split_whitespace().collect();
    let runs1: u32 = parts[4].parse().unwrap();
    println!("Unix echo initial runs: {}", runs1);
    assert!(runs1 >= 1, "Should have run at least once");

    // Step 3: Process already exited after handling request (Accept=no mode behavior)
    println!("Unix socket service handled request and exited");

    // Give PM time to detect process exit and reset socket state
    std::thread::sleep(Duration::from_secs(2));

    // Step 4: Re-activate via Unix socket
    println!("Re-activating via Unix socket...");
    std::thread::sleep(Duration::from_millis(500)); // Give socket time to reset
    let mut stream = UnixStream::connect(&socket_path).expect("Should re-connect to Unix socket");
    stream
        .set_read_timeout(Some(Duration::from_secs(5)))
        .unwrap();

    stream.write_all(b"Reactivated\n").unwrap();

    let mut buffer = [0u8; 1024];
    let n = stream.read(&mut buffer).unwrap();
    let response = String::from_utf8_lossy(&buffer[..n]);
    println!("Re-activated Unix response: {}", response);
    assert!(response.contains("Reactivated"));
    drop(stream);

    std::thread::sleep(Duration::from_millis(500));

    // Step 5: Verify re-activation by checking run count
    let (list_out, _, _) = common::run_cli_full(&["list"]);
    let echo_line = list_out.lines().find(|l| l.contains("echo-unix")).unwrap();
    let parts: Vec<&str> = echo_line.split_whitespace().collect();
    let runs2: u32 = parts[4].parse().unwrap();
    println!("Re-activated Unix echo runs: {}", runs2);

    assert_eq!(
        runs2,
        runs1 + 1,
        "Unix socket re-activation should trigger new run"
    );

    println!("[OK] Unix socket re-activation SUCCESS!");

    // Cleanup
    // Daemon cleanup handled by guard
}

#[test]
fn test_socket_activation_multiple_crashes_and_reactivations() {
    // Use unique port for this test
    let test_socket_port = common::get_socket_test_port() + 2000; // Different offset

    let script_path =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("socket_echo_server.py");

    let process_config = format!(
        r#"
command: /usr/bin/python3
args: ["{}"]
working_dir: /tmp
auto_start: false
process_type: simple
restart: never
stdout: /tmp/multi-echo-stdout.log
stderr: /tmp/multi-echo-stderr.log
"#,
        script_path.display()
    );

    let socket_config = format!(
        r#"
listen_stream: "127.0.0.1:{}"
service: multi-echo
accept: false
"#,
        test_socket_port
    );

    let config_dir = create_socket_config_dir("multi-echo", "multi-tcp", &process_config, &socket_config);

    let _daemon = common::setup_daemon_with_config_dir(&config_dir);

    // Give daemon time to initialize sockets
    std::thread::sleep(Duration::from_secs(3));

    // Verify socket was created
    let (list_out, _, _) = common::run_cli_full(&["list"]);
    println!("Process list:\n{}", list_out);

    // Check if socket is actually listening
    println!("Attempting to connect to 127.0.0.1:{}...", test_socket_port);

    let mut previous_runs = 0u32;

    // Test multiple crash/re-activation cycles
    for cycle in 1..=3 {
        println!("\n=== Cycle {} ===", cycle);

        // Trigger activation
        let mut stream = TcpStream::connect(format!("127.0.0.1:{}", test_socket_port))
            .unwrap_or_else(|_| panic!("Failed to connect to socket on port {}", test_socket_port));
        stream
            .set_read_timeout(Some(Duration::from_secs(5)))
            .unwrap();

        let msg = format!("Cycle{}\n", cycle);
        stream.write_all(msg.as_bytes()).unwrap();

        let mut buffer = [0u8; 1024];
        let n = stream.read(&mut buffer).unwrap();
        let response = String::from_utf8_lossy(&buffer[..n]);
        println!("Response: {}", response);
        assert!(response.contains(&format!("Cycle{}", cycle)));
        drop(stream);

        std::thread::sleep(Duration::from_millis(500));

        // Check run count
        let (list_out, _, _) = common::run_cli_full(&["list"]);
        let echo_line = list_out.lines().find(|l| l.contains("multi-echo")).unwrap();
        let parts: Vec<&str> = echo_line.split_whitespace().collect();
        let current_runs: u32 = parts[4].parse().unwrap();
        println!("Current runs: {}", current_runs);

        if cycle > 1 {
            assert_eq!(
                current_runs,
                previous_runs + 1,
                "Each cycle should increment runs"
            );
        }

        // Process already exited after handling request
        println!("Cycle {} complete, process exited", cycle);

        // Give PM time to detect process exit and reset socket for next cycle
        if cycle < 3 {
            std::thread::sleep(Duration::from_secs(2));
        }

        previous_runs = current_runs;
    }

    println!("\n[OK] Successfully completed 3 crash/re-activation cycles");

    // Cleanup
    // Daemon cleanup handled by guard
}
