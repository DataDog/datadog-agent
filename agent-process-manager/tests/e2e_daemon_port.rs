use pm_e2e_tests::{get_daemon_binary, run_cli_port, unique_socket_path};
use std::process::{Command, Stdio};
use std::thread;
use std::time::Duration;

#[test]
fn test_daemon_respects_custom_port() {
    let custom_port = 55123;

    // Start daemon with explicit port
    let mut daemon = Command::new(get_daemon_binary())
        .arg("--tcp") // New daemon defaults to Unix socket
        .arg("--grpc-port")
        .arg(custom_port.to_string())
        .arg("--grpc-socket")
        .arg(unique_socket_path(custom_port))
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("Failed to start daemon");

    thread::sleep(Duration::from_secs(1));

    // Verify it's listening on the specified port
    let (stdout, stderr, code) = run_cli_port(&["list"], custom_port);
    assert_eq!(code, 0, "Should connect to port {}", custom_port);
    assert!(
        stdout.contains("NAME") || stdout.contains("No processes"),
        "Should get valid list output. stdout: {}, stderr: {}",
        stdout,
        stderr
    );

    // Try wrong port - should fail to connect
    let (_, stderr, code) = run_cli_port(&["list"], custom_port + 1);
    assert_ne!(
        code, 0,
        "Should NOT connect to wrong port. stderr: {}",
        stderr
    );

    // Cleanup
    let _ = daemon.kill();
    let _ = daemon.wait();
    thread::sleep(Duration::from_millis(100));
}

#[test]
fn test_daemon_default_port() {
    // Start daemon without --grpc-port flag (should use default 50051)
    let mut daemon = Command::new(get_daemon_binary())
        .arg("--tcp") // Must use TCP mode to test port
        .arg("--grpc-socket")
        .arg(unique_socket_path(50051))
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("Failed to start daemon");

    thread::sleep(Duration::from_secs(1));

    // Verify it's listening on default port
    let (stdout, _, code) = run_cli_port(&["list"], 50051);
    assert_eq!(code, 0, "Should connect to default port 50051");
    assert!(
        stdout.contains("NAME") || stdout.contains("No processes"),
        "Should get valid list output on default port"
    );

    // Cleanup
    let _ = daemon.kill();
    let _ = daemon.wait();
    thread::sleep(Duration::from_millis(100));
}

#[test]
fn test_daemon_port_via_env_var() {
    let custom_port = 55124;

    // Start daemon with port from env var
    let mut daemon = Command::new(get_daemon_binary())
        .arg("--tcp") // Must use TCP mode to test port
        .arg("--grpc-socket")
        .arg(unique_socket_path(custom_port))
        .env("DD_PM_DAEMON_PORT", custom_port.to_string())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("Failed to start daemon");

    thread::sleep(Duration::from_secs(1));

    // Verify it's listening on the env var port
    let (stdout, _, code) = run_cli_port(&["list"], custom_port);
    assert_eq!(code, 0, "Should connect to port from env var");
    assert!(
        stdout.contains("NAME") || stdout.contains("No processes"),
        "Should get valid list output"
    );

    // Cleanup
    let _ = daemon.kill();
    let _ = daemon.wait();
    thread::sleep(Duration::from_millis(100));
}

#[test]
fn test_daemon_cli_port_overrides_env_var() {
    let cli_port = 55125;
    let env_port = 55126;

    // Start daemon with both CLI arg and env var (CLI should win)
    let mut daemon = Command::new(get_daemon_binary())
        .arg("--tcp") // Must use TCP mode to test port
        .arg("--grpc-port")
        .arg(cli_port.to_string())
        .arg("--grpc-socket")
        .arg(unique_socket_path(cli_port))
        .env("DD_PM_DAEMON_PORT", env_port.to_string())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("Failed to start daemon");

    thread::sleep(Duration::from_secs(1));

    // Verify it's listening on CLI port (not env var port)
    let (stdout, _, code) = run_cli_port(&["list"], cli_port);
    assert_eq!(code, 0, "Should connect to CLI port");
    assert!(
        stdout.contains("NAME") || stdout.contains("No processes"),
        "Should get valid list output"
    );

    // Env var port should NOT work
    let (_, _, code) = run_cli_port(&["list"], env_port);
    assert_ne!(code, 0, "Should NOT connect to env var port");

    // Cleanup
    let _ = daemon.kill();
    let _ = daemon.wait();
    thread::sleep(Duration::from_millis(100));
}
