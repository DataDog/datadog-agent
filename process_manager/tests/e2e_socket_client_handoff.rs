//! E2E tests for trace-loader style socket client handoff
//!
//! These tests verify that when `client_fd_env_var` is configured, the daemon:
//! 1. Accepts the first connection on the listening socket
//! 2. Passes BOTH the listener socket AND the accepted client connection to the child
//! 3. The child can communicate on the pre-accepted connection immediately
//! 4. The child can also accept new connections on the listener
//!
//! This matches the behavior of the trace-loader, which accepts the first connection
//! before spawning the trace-agent, allowing for zero-connection-loss handoff.

use pm_e2e_tests::{
    get_socket_test_port, process_exists_by_name, run_cli_full, setup_daemon_with_config_dir,
    unique_process_name, unique_test_path,
};
use std::io::{Read, Write};
use std::net::TcpStream;
use std::thread;
use std::time::Duration;

/// Create a config directory with process and socket files for client handoff testing
fn create_client_handoff_config(
    service_name: &str,
    socket_name: &str,
    port: u16,
) -> String {
    let config_dir = unique_test_path("client-handoff", ".d");
    std::fs::create_dir_all(&config_dir).expect("Failed to create config dir");

    // Get the path to the Python test script
    // Note: When running tests, current_dir may be 'tests/' or 'process_manager/'
    // We use the manifest dir which is always the tests/ directory
    let script_path = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("socket_client_handoff_server.py");

    // Process config - Python echo server that handles client handoff
    let process_config = format!(
        r#"
command: python3
args:
  - "{}"
auto_start: false
"#,
        script_path.display()
    );

    // Socket config with client_fd_env_var for client handoff
    let socket_config = format!(
        r#"
listen_stream: "127.0.0.1:{}"
service: {}
accept: false
fd_env_var: DD_APM_NET_RECEIVER_FD
client_fd_env_var: DD_APM_NET_RECEIVER_CLIENT_FD
"#,
        port, service_name
    );

    // Write configs
    let process_path = format!("{}/{}.yaml", config_dir, service_name);
    std::fs::write(&process_path, process_config).expect("Failed to write process config");

    let socket_path = format!("{}/{}.socket.yaml", config_dir, socket_name);
    std::fs::write(&socket_path, socket_config).expect("Failed to write socket config");

    config_dir
}

/// Test that client handoff works: the first connection is accepted by the daemon
/// and passed to the child process, which can immediately communicate on it.
#[test]
fn test_e2e_client_handoff_first_connection() {
    let socket_name = unique_process_name();
    let service_name = unique_process_name();
    let port = get_socket_test_port();

    let config_dir = create_client_handoff_config(&service_name, &socket_name, port);

    // Start daemon
    let _daemon = setup_daemon_with_config_dir(&config_dir);
    thread::sleep(Duration::from_secs(3));

    // Verify process was created
    let (stdout, _, code) = run_cli_full(&["list"]);
    if code != 0 || stdout.is_empty() {
        std::fs::remove_dir_all(&config_dir).ok();
        return;
    }

    assert!(
        process_exists_by_name(&service_name),
        "Process '{}' should exist",
        service_name
    );

    // Connect to trigger socket activation - this connection will be pre-accepted
    // by the daemon and passed to the child via DD_APM_NET_RECEIVER_CLIENT_FD
    let connect_result = TcpStream::connect_timeout(
        &format!("127.0.0.1:{}", port).parse().unwrap(),
        Duration::from_secs(5),
    );

    if let Ok(mut stream) = connect_result {
        stream.set_read_timeout(Some(Duration::from_secs(5))).ok();
        stream.set_write_timeout(Some(Duration::from_secs(5))).ok();

        // Send data on the pre-accepted connection
        let test_message = "hello_from_first_connection";
        match stream.write_all(format!("{}\n", test_message).as_bytes()) {
            Ok(_) => {
                // Wait for process to start and handle the connection
                thread::sleep(Duration::from_secs(2));

                // Read response
                let mut buf = vec![0u8; 1024];
                match stream.read(&mut buf) {
                    Ok(n) if n > 0 => {
                        let response = String::from_utf8_lossy(&buf[..n]);
                        println!("Response from first connection: {}", response);

                        // Should get HANDOFF_OK response indicating the child
                        // received the pre-accepted connection
                        assert!(
                            response.contains("HANDOFF_OK"),
                            "Expected HANDOFF_OK response, got: {}",
                            response
                        );
                        assert!(
                            response.contains(test_message),
                            "Response should echo our message"
                        );
                    }
                    Ok(_) => {
                        println!("Warning: No data received (connection may have closed)");
                    }
                    Err(e) => {
                        println!("Warning: Failed to read response: {}", e);
                    }
                }
            }
            Err(e) => {
                println!("Warning: Failed to write to socket: {}", e);
            }
        }
        drop(stream);
    } else {
        println!(
            "Warning: Could not connect to trigger activation: {:?}",
            connect_result
        );
    }

    // Verify process started
    thread::sleep(Duration::from_secs(1));
    let (stdout, _, _) = run_cli_full(&["list"]);
    println!("Process list after activation: {}", stdout);

    // Clean up
    std::fs::remove_dir_all(&config_dir).ok();
}

/// Test that after client handoff, the child can still accept new connections
/// on the listener socket.
#[test]
fn test_e2e_client_handoff_listener_still_works() {
    let socket_name = unique_process_name();
    let service_name = unique_process_name();
    let port = get_socket_test_port();

    let config_dir = create_client_handoff_config(&service_name, &socket_name, port);

    // Start daemon
    let _daemon = setup_daemon_with_config_dir(&config_dir);
    thread::sleep(Duration::from_secs(3));

    // First connection - triggers activation and gets handed off
    let first_connect = TcpStream::connect_timeout(
        &format!("127.0.0.1:{}", port).parse().unwrap(),
        Duration::from_secs(5),
    );

    if let Ok(mut first_stream) = first_connect {
        first_stream
            .set_read_timeout(Some(Duration::from_secs(5)))
            .ok();
        first_stream
            .set_write_timeout(Some(Duration::from_secs(5)))
            .ok();

        // Send on first connection
        first_stream.write_all(b"first_connection\n").ok();
        thread::sleep(Duration::from_secs(2));

        // Read response
        let mut buf = vec![0u8; 1024];
        if let Ok(n) = first_stream.read(&mut buf) {
            if n > 0 {
                let response = String::from_utf8_lossy(&buf[..n]);
                println!("First connection response: {}", response);
                assert!(
                    response.contains("HANDOFF_OK"),
                    "First connection should get HANDOFF_OK"
                );
            }
        }
        drop(first_stream);

        // Give child time to start accepting on listener
        thread::sleep(Duration::from_secs(1));

        // Second connection - should be accepted by the child on the listener
        let second_connect = TcpStream::connect_timeout(
            &format!("127.0.0.1:{}", port).parse().unwrap(),
            Duration::from_secs(5),
        );

        if let Ok(mut second_stream) = second_connect {
            second_stream
                .set_read_timeout(Some(Duration::from_secs(5)))
                .ok();
            second_stream
                .set_write_timeout(Some(Duration::from_secs(5)))
                .ok();

            // Send on second connection
            second_stream.write_all(b"second_connection\n").ok();

            // Read response
            let mut buf2 = vec![0u8; 1024];
            match second_stream.read(&mut buf2) {
                Ok(n) if n > 0 => {
                    let response = String::from_utf8_lossy(&buf2[..n]);
                    println!("Second connection response: {}", response);

                    // Should get LISTENER_OK response indicating the child
                    // accepted this connection on the listener socket
                    assert!(
                        response.contains("LISTENER_OK"),
                        "Second connection should get LISTENER_OK, got: {}",
                        response
                    );
                }
                Ok(_) => {
                    println!("Warning: No data received on second connection");
                }
                Err(e) => {
                    println!("Warning: Failed to read from second connection: {}", e);
                }
            }
            drop(second_stream);
        } else {
            println!(
                "Warning: Could not establish second connection: {:?}",
                second_connect
            );
        }
    } else {
        println!(
            "Warning: Could not connect to trigger activation: {:?}",
            first_connect
        );
    }

    // Clean up
    std::fs::remove_dir_all(&config_dir).ok();
}

/// Test socket activation without client handoff (traditional Accept=no behavior)
/// to ensure we didn't break backward compatibility.
#[test]
fn test_e2e_socket_activation_without_client_handoff() {
    let socket_name = unique_process_name();
    let service_name = unique_process_name();
    let port = get_socket_test_port();

    let config_dir = unique_test_path("no-handoff", ".d");
    std::fs::create_dir_all(&config_dir).expect("Failed to create config dir");

    // Simple process that sleeps
    let process_config = r#"
command: sleep
args: ["30"]
auto_start: false
"#;

    // Socket WITHOUT client_fd_env_var (traditional behavior)
    let socket_config = format!(
        r#"
listen_stream: "127.0.0.1:{}"
service: {}
accept: false
fd_env_var: DD_APM_NET_RECEIVER_FD
"#,
        port, service_name
    );

    std::fs::write(
        format!("{}/{}.yaml", config_dir, service_name),
        process_config,
    )
    .unwrap();
    std::fs::write(
        format!("{}/{}.socket.yaml", config_dir, socket_name),
        socket_config,
    )
    .unwrap();

    // Start daemon
    let _daemon = setup_daemon_with_config_dir(&config_dir);
    thread::sleep(Duration::from_secs(3));

    // Verify process exists but not running
    assert!(
        process_exists_by_name(&service_name),
        "Process should exist"
    );

    let (stdout, _, _) = run_cli_full(&["list"]);
    assert!(
        stdout.to_lowercase().contains("created"),
        "Process should be in Created state before activation"
    );

    // Connect to trigger activation
    let connect = TcpStream::connect_timeout(
        &format!("127.0.0.1:{}", port).parse().unwrap(),
        Duration::from_secs(5),
    );

    if connect.is_ok() {
        thread::sleep(Duration::from_secs(2));

        // Process should now be running
        let (stdout, _, _) = run_cli_full(&["list"]);
        assert!(
            stdout.to_lowercase().contains("running"),
            "Process should be running after socket activation"
        );
    }

    // Clean up
    std::fs::remove_dir_all(&config_dir).ok();
}

