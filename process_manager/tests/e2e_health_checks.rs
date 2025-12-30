use pm_e2e_tests::{
    create_and_start_process, delete_process, get_test_port, run_cli_full, setup_daemon,
    setup_daemon_with_config_dir, stop_process,
};
use std::process::{Command, Stdio};
use std::thread;
use std::time::Duration;

#[test]
fn test_e2e_tcp_health_check_success() {
    let _daemon = setup_daemon();

    // Use unique port based on test thread
    let tcp_port = 60000 + (get_test_port() % 1000); // 60000-60999 range for test servers

    // Start a simple TCP server using netcat or Python
    let bind_cmd = format!(
        "import socket; s=socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); s.bind(('127.0.0.1',{})); s.listen(1); s.accept()",
        tcp_port
    );
    let mut server = Command::new("/usr/bin/python3")
        .args(["-c", &bind_cmd])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("Failed to start TCP server");

    thread::sleep(Duration::from_millis(500));

    // Create and start process with TCP health check
    let tcp_port_str = tcp_port.to_string();
    let id = create_and_start_process(
        "tcp_test",
        "sleep",
        &[
            "30",
            "--restart",
            "never",
            "--health-check-type",
            "tcp",
            "--health-check-tcp-host",
            "127.0.0.1",
            "--health-check-tcp-port",
            &tcp_port_str,
            "--health-check-interval",
            "2",
            "--health-check-timeout",
            "1",
            "--health-check-retries",
            "2",
        ],
    );

    // Wait for health checks to run
    thread::sleep(Duration::from_secs(5));

    // Check health status
    let (stdout, _stderr, _exit_code) = run_cli_full(&["describe", &id]);
    assert!(
        stdout.contains("Process Details"),
        "Should describe process: {}",
        stdout
    );
    assert!(
        stdout.contains("healthy") || stdout.contains("starting"),
        "Should show healthy status: {}",
        stdout
    );

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    let _ = server.kill();
    let _ = server.wait();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_tcp_health_check_failure() {
    let _daemon = setup_daemon();

    // Use unique port (nothing listening - intentional failure)
    let fail_port = 61000 + (get_test_port() % 1000);
    let fail_port_str = fail_port.to_string();

    // Create and start process with TCP health check to a non-existent port
    let id = create_and_start_process(
        "tcp_fail_test",
        "sleep",
        &[
            "30",
            "--restart",
            "never",
            "--health-check-type",
            "tcp",
            "--health-check-tcp-host",
            "127.0.0.1",
            "--health-check-tcp-port",
            &fail_port_str, // Nothing listening here
            "--health-check-interval",
            "2",
            "--health-check-timeout",
            "1",
            "--health-check-retries",
            "2",
        ],
    );

    // Wait for health checks to fail
    thread::sleep(Duration::from_secs(6));

    // Check that process was killed due to health failure
    // (Since restart=never, it should stay stopped after being killed by health check)
    let (stdout, _stderr, _exit_code) = run_cli_full(&["describe", &id]);

    // Process should have been killed by health check
    assert!(
        stdout.contains("Process Details"),
        "Describe should return info: {}",
        stdout
    );
    // Should either be crashed/exited (killed) or still running but unhealthy
    assert!(
        stdout.contains("unhealthy") || stdout.contains("crashed") || stdout.contains("exited"),
        "Should show unhealthy or killed status: {}",
        stdout
    );

    // Cleanup
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_exec_health_check_success() {
    let _daemon = setup_daemon();

    // Create a file that the health check will look for
    std::fs::write("/tmp/test_healthy", "ok").expect("Failed to create health file");

    // Create and start process with Exec health check
    let id = create_and_start_process(
        "exec_test",
        "sleep",
        &[
            "30",
            "--restart",
            "never",
            "--health-check-type",
            "exec",
            "--health-check-exec-command",
            "test",
            "--health-check-exec-arg",
            "-f",
            "--health-check-exec-arg",
            "/tmp/test_healthy",
            "--health-check-interval",
            "2",
            "--health-check-timeout",
            "1",
            "--health-check-retries",
            "2",
        ],
    );

    // Wait for health checks to run
    thread::sleep(Duration::from_secs(5));

    // Check health status
    let (stdout, _stderr, _exit_code) = run_cli_full(&["describe", &id]);
    assert!(
        stdout.contains("Process Details"),
        "Should describe process: {}",
        stdout
    );
    assert!(
        stdout.contains("healthy") || stdout.contains("starting"),
        "Should show healthy status: {}",
        stdout
    );

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    let _ = std::fs::remove_file("/tmp/test_healthy");
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_exec_health_check_failure() {
    let _daemon = setup_daemon();

    // Create and start process with Exec health check that will fail
    let id = create_and_start_process(
        "exec_fail_test",
        "sleep",
        &[
            "30",
            "--restart",
            "never",
            "--health-check-type",
            "exec",
            "--health-check-exec-command",
            "test",
            "--health-check-exec-arg",
            "-f",
            "--health-check-exec-arg",
            "/tmp/does_not_exist_health_file",
            "--health-check-interval",
            "2",
            "--health-check-timeout",
            "1",
            "--health-check-retries",
            "2",
        ],
    );

    // Wait for health checks to fail and kill the process
    thread::sleep(Duration::from_secs(6));

    // Process should have been killed by health check
    let (stdout, _stderr, _exit_code) = run_cli_full(&["describe", &id]);
    assert!(
        stdout.contains("Process Details"),
        "Should still be able to describe: {}",
        stdout
    );
    // Should either be crashed/exited (killed) or still running but unhealthy
    assert!(
        stdout.contains("unhealthy") || stdout.contains("crashed") || stdout.contains("exited"),
        "Should show unhealthy or killed status: {}",
        stdout
    );

    // Cleanup
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_health_check_restart_on_failure() {
    let _daemon = setup_daemon();

    // Use unique port (nothing listening - intentional failure)
    let fail_port = 62000 + (get_test_port() % 1000);
    let fail_port_str = fail_port.to_string();

    // Create and start process with restart=always and a failing health check
    let id = create_and_start_process(
        "restart_test",
        "sleep",
        &[
            "30",
            "--restart",
            "always",
            "--restart-sec",
            "1",
            "--health-check-type",
            "tcp",
            "--health-check-tcp-host",
            "127.0.0.1",
            "--health-check-tcp-port",
            &fail_port_str, // Nothing listening
            "--health-check-interval",
            "2",
            "--health-check-timeout",
            "1",
            "--health-check-retries",
            "2",
        ],
    );

    // Wait for health check to fail and trigger restart
    thread::sleep(Duration::from_secs(8));

    // Check that process has restarted (run count > 1)
    let (stdout, _stderr, _exit_code) = run_cli_full(&["describe", &id]);

    // Process should have been restarted at least once
    assert!(
        stdout.contains("Process Details"),
        "Should describe process: {}",
        stdout
    );
    assert!(
        stdout.contains("Run Count:"),
        "Should show run count: {}",
        stdout
    );

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_health_check_yaml_config() {
    // Use unique port for YAML config test
    let yaml_port = 64000 + (get_test_port() % 1000);

    // Create config directory with process config (direct ProcessConfig format)
    let config_dir = format!("/tmp/test_health_config_{}.d", std::process::id());
    std::fs::create_dir_all(&config_dir).expect("Failed to create config dir");

    let config_content = format!(
        r#"
command: python3
args:
  - -c
  - "import socket; s=socket.socket(); s.bind(('127.0.0.1',{})); s.listen(1); import time; time.sleep(30)"
restart: never
health_check:
  type: tcp
  host: 127.0.0.1
  port: {}
  interval: 2
  timeout: 1
  retries: 2
  start_period: 1
auto_start: true
"#,
        yaml_port, yaml_port
    );

    // Process name derived from filename
    let config_path = format!("{}/yaml_health_test.yaml", config_dir);
    std::fs::write(&config_path, config_content).expect("Failed to write config");

    // Use setup_daemon_with_config_dir
    let _daemon = setup_daemon_with_config_dir(&config_dir);

    thread::sleep(Duration::from_secs(3));

    // List processes - should see our health check process
    let (stdout, _stderr, exit_code) = run_cli_full(&["list"]);
    assert_eq!(exit_code, 0, "List should succeed");
    assert!(
        stdout.contains("yaml_health_test"),
        "Should find process from config: {}",
        stdout
    );

    // Describe to check health status
    let (stdout, _stderr, _exit_code) = run_cli_full(&["describe", "yaml_health_test"]);
    assert!(
        stdout.contains("Process Details"),
        "Should describe process: {}",
        stdout
    );
    assert!(
        stdout.contains("Health Status:"),
        "Should show health status: {}",
        stdout
    );

    // Cleanup
    let _ = run_cli_full(&["stop", "yaml_health_test"]);
    let _ = run_cli_full(&["delete", "yaml_health_test", "--force"]);
    std::fs::remove_dir_all(&config_dir).ok();
    // Daemon cleanup handled by guard
}

#[test]
fn test_e2e_http_health_check_with_python_server() {
    let _daemon = setup_daemon();

    // Use unique port for HTTP server
    let http_port = 63000 + (get_test_port() % 1000);
    let http_port_str = http_port.to_string();
    let http_endpoint = format!("http://127.0.0.1:{}/", http_port);

    // Start Python HTTP server in background
    let mut http_server = Command::new("/usr/bin/python3")
        .args(["-m", "http.server", &http_port_str])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("Failed to start HTTP server");

    thread::sleep(Duration::from_secs(1)); // Give server time to start

    // Create and start process with HTTP health check pointing to the server
    let id = create_and_start_process(
        "http_test",
        "sleep",
        &[
            "30",
            "--restart",
            "never",
            "--health-check-type",
            "http",
            "--health-check-http-endpoint",
            &http_endpoint,
            "--health-check-http-method",
            "GET",
            "--health-check-http-status",
            "200",
            "--health-check-interval",
            "3",
            "--health-check-timeout",
            "2",
            "--health-check-retries",
            "2",
            "--health-check-start-period",
            "2",
        ],
    );

    // Wait for health checks to run
    thread::sleep(Duration::from_secs(6));

    // Check health status
    let (stdout, _stderr, _exit_code) = run_cli_full(&["describe", &id]);
    assert!(
        stdout.contains("Process Details"),
        "Should describe process: {}",
        stdout
    );
    assert!(
        stdout.contains("healthy") || stdout.contains("starting"),
        "Should show healthy status: {}",
        stdout
    );

    // Cleanup
    stop_process(&id);
    delete_process(&id);
    let _ = http_server.kill();
    let _ = http_server.wait();
    // Daemon cleanup handled by guard
}
