//! Shared test utilities for E2E tests
//!
//! ## Daemon Binary Selection
//!
//! Tests use the daemon by default (`dd-procmgrd`).
//! Can be overridden via PM_DAEMON_BINARY environment variable for testing.
//!
//! ## Test Isolation
//!
//! Each test runs with its own daemon instance on a unique port to enable
//! parallel test execution without interference. This is achieved through:
//!
//! - **Atomic Port Counter**: Ensures each test gets a unique port (50000+)
//! - **Thread-Local Storage**: Each test thread stores its assigned port and daemon PID
//! - **Automatic Cleanup**: `cleanup_daemon()` kills only the daemon for that test
//!
//! ## Usage Pattern
//!
//! ```rust,ignore
//! #[test]
//! fn my_test() {
//!     let _daemon = setup_daemon();
//!     // Test runs with isolated daemon
//!     // Daemon automatically cleaned up when guard drops (even on panic!)
//! }
//! ```
//!
//! ## Automatic Daemon Log Printing on Test Failure (CI-Friendly!)
//!
//! When any test panics/fails, the last 50 lines of daemon logs are **automatically**
//! printed to stderr. This works for ALL E2E tests without any manual intervention.
//!
//! **How it works:**
//! - A panic hook is installed when the common library loads (via `#[ctor]`)
//! - Each test binary links against common.rs, so the hook is installed automatically
//! - When a test panics, the hook checks thread-local storage for daemon log paths
//! - If found, it prints the last 50 lines of stdout/stderr to help debug failures
//!
//! **This is especially useful in CI** where you can't manually check log files!
//!
//! Daemon logs are captured to /tmp/daemon-{port}-{pid}.{stdout,stderr}.log files.
//!
//! **Manual log printing (if needed):**
//! ```rust,ignore
//! #[test]
//! fn my_test() {
//!     let _daemon = setup_daemon();
//!     // ... test code ...
//!     print_daemon_logs();  // Call this to print full logs at any point
//!     print_daemon_logs_tail(100);  // Or print last N lines
//! }
//! ```

use std::cell::RefCell;
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicU16, Ordering};
use std::sync::Once;
use std::thread;
use std::time::Duration;

/// Test port allocation constants (previously from pm_engine::constants::test)
pub const BASE_PORT: u16 = 50000;
pub const PORT_RANGE: u16 = 10000;
pub const DEFAULT_PORT: u16 = 50051;

// Install panic hook globally when this library is loaded
// This ensures ALL E2E tests get automatic daemon log printing on failure
//
// The #[ctor] attribute ensures this function runs BEFORE any test starts,
// when the test binary loads the common library. This happens automatically
// for every E2E test file since they all link against common.rs as a library.
#[ctor::ctor]
fn init_panic_hook() {
    install_panic_hook();
}

/// Global atomic counter for allocating unique ports to tests
static PORT_COUNTER: AtomicU16 = AtomicU16::new(0);

/// Ensure panic hook is installed only once across all tests
static PANIC_HOOK_INIT: Once = Once::new();

// Thread-local storage for port, daemon PID, log files, and transport mode assigned to each test
thread_local! {
    static TEST_PORT: RefCell<Option<u16>> = const { RefCell::new(None) };
    static DAEMON_PID: RefCell<Option<u32>> = const { RefCell::new(None) };
    static DAEMON_LOG_FILES: RefCell<Option<(String, String)>> = const { RefCell::new(None) };
    static TRANSPORT_MODE: RefCell<Option<bool>> = const { RefCell::new(None) }; // Some(true) = TCP, Some(false) = Unix, None = unknown
}

/// Install panic hook to automatically print daemon logs on test failure
///
/// This is called once globally, ensuring ALL tests that panic will
/// automatically print daemon logs if available (via thread-local storage).
///
/// The hook is installed on first daemon setup, but applies to all subsequent tests.
fn install_panic_hook() {
    PANIC_HOOK_INIT.call_once(|| {
        let default_hook = std::panic::take_hook();
        std::panic::set_hook(Box::new(move |panic_info| {
            // Print daemon logs if available for this test thread
            DAEMON_LOG_FILES.with(|logs| {
                if let Some((stdout_path, stderr_path)) = logs.borrow().as_ref() {
                    eprintln!("\n========== DAEMON LOGS (Test Failed) ==========");
                    eprintln!("Daemon stdout: {}", stdout_path);
                    eprintln!("Daemon stderr: {}", stderr_path);

                    // Print last 50 lines of each log
                    if let Ok(stdout_content) = std::fs::read_to_string(stdout_path) {
                        let lines: Vec<&str> = stdout_content.lines().collect();
                        let start = lines.len().saturating_sub(50);
                        eprintln!("\n--- Last 50 lines of STDOUT ---");
                        for line in &lines[start..] {
                            eprintln!("{}", line);
                        }
                    }

                    if let Ok(stderr_content) = std::fs::read_to_string(stderr_path) {
                        let lines: Vec<&str> = stderr_content.lines().collect();
                        let start = lines.len().saturating_sub(50);
                        eprintln!("\n--- Last 50 lines of STDERR ---");
                        for line in &lines[start..] {
                            eprintln!("{}", line);
                        }
                    }
                    eprintln!("===============================================\n");
                }
            });

            // Call the default panic hook
            default_hook(panic_info);
        }));
    });
}

/// Get the daemon binary path
///
/// Returns the daemon binary path (`dd-procmgrd`).
/// Can be overridden via PM_DAEMON_BINARY environment variable for testing.
pub fn get_daemon_binary() -> &'static str {
    // Allow override for testing with different daemon binaries
    if let Ok(binary) = std::env::var("PM_DAEMON_BINARY") {
        // Leak the string to get a 'static reference
        // This is acceptable in tests as it only happens once
        Box::leak(binary.into_boxed_str())
    } else {
        // Default daemon binary (use debug for faster iteration)
        "../target/debug/dd-procmgrd"
    }
}

/// Get the CLI binary path
///
/// Returns the CLI binary path.
/// Can be overridden via PM_CLI_BINARY environment variable for testing.
pub fn get_cli_binary() -> &'static str {
    if let Ok(binary) = std::env::var("PM_CLI_BINARY") {
        Box::leak(binary.into_boxed_str())
    } else {
        "../target/debug/dd-procmgr"
    }
}

/// RAII guard that ensures daemon cleanup even on panic
/// When this guard is dropped (goes out of scope), it automatically calls cleanup_daemon()
#[must_use = "DaemonGuard must be held for the duration of the test"]
pub struct DaemonGuard {
    _private: (),
}

#[allow(dead_code)]
impl DaemonGuard {
    /// Get daemon stdout logs from thread-local storage
    pub fn stdout_logs(&self) -> String {
        DAEMON_LOG_FILES.with(|logs| {
            if let Some((stdout_path, _)) = logs.borrow().as_ref() {
                std::fs::read_to_string(stdout_path).unwrap_or_default()
            } else {
                String::new()
            }
        })
    }

    /// Get daemon stderr logs from thread-local storage
    pub fn stderr_logs(&self) -> String {
        DAEMON_LOG_FILES.with(|logs| {
            if let Some((_, stderr_path)) = logs.borrow().as_ref() {
                std::fs::read_to_string(stderr_path).unwrap_or_default()
            } else {
                String::new()
            }
        })
    }

    /// Get the port the daemon is listening on from thread-local storage
    pub fn port(&self) -> u16 {
        TEST_PORT.with(|port| port.borrow().unwrap_or(0))
    }
}

impl Drop for DaemonGuard {
    fn drop(&mut self) {
        cleanup_daemon();
    }
}

/// Get a unique port for this test
/// Uses an atomic counter to ensure each call gets a different port
fn get_unique_port() -> u16 {
    // Check if this thread already has a port assigned
    TEST_PORT.with(|port| {
        let mut port_ref = port.borrow_mut();
        if let Some(p) = *port_ref {
            return p;
        }

        // Allocate a new port for this thread
        let counter = PORT_COUNTER.fetch_add(1, Ordering::SeqCst);
        let new_port = BASE_PORT + (counter % PORT_RANGE);
        *port_ref = Some(new_port);
        new_port
    })
}

/// Get the port assigned to this test thread
/// Returns the port if one was assigned, otherwise returns default
#[allow(dead_code)]
pub fn get_test_port() -> u16 {
    TEST_PORT.with(|p| p.borrow().unwrap_or(DEFAULT_PORT))
}

/// Generate a unique file path for this test thread
/// Useful for avoiding file conflicts in parallel tests
#[allow(dead_code)]
pub fn unique_test_path(prefix: &str, suffix: &str) -> String {
    // Use get_unique_port() to ensure port is allocated for this thread
    // This prevents mismatches where config files use DEFAULT_PORT (50051)
    // but daemon runs on a different allocated port
    let port = get_unique_port();
    format!("/tmp/{}_{}{}", prefix, port, suffix)
}

/// Generate a unique Unix socket path for tests
/// Uses port and process ID to avoid conflicts in parallel test execution
#[allow(dead_code)]
pub fn unique_socket_path(port: u16) -> String {
    format!("/tmp/pm-test-{}-{}.sock", port, std::process::id())
}

/// Builder for waiting on daemon health with flexible transport options
#[allow(dead_code)]
pub struct DaemonHealthCheck {
    port: Option<u16>,
    socket: Option<String>,
    timeout_secs: u64,
}

#[allow(dead_code)]
impl Default for DaemonHealthCheck {
    fn default() -> Self {
        Self::new()
    }
}

#[allow(dead_code)]
impl DaemonHealthCheck {
    /// Create a new health check builder
    pub fn new() -> Self {
        Self {
            port: None,
            socket: None,
            timeout_secs: 10,
        }
    }

    /// Set TCP port for health check
    pub fn port(mut self, port: u16) -> Self {
        self.port = Some(port);
        self.socket = None; // Clear socket if port is set
        self
    }

    /// Set Unix socket path for health check
    pub fn socket(mut self, socket: &str) -> Self {
        self.socket = Some(socket.to_string());
        self.port = None; // Clear port if socket is set
        self
    }

    /// Set timeout in seconds (default: 10)
    pub fn timeout(mut self, timeout_secs: u64) -> Self {
        self.timeout_secs = timeout_secs;
        self
    }

    /// Wait for daemon to become healthy
    /// Returns true if daemon is healthy, false if timeout
    pub fn wait(self) -> bool {
        let cli_binary = get_cli_binary();
        let start = std::time::Instant::now();
        let timeout = Duration::from_secs(self.timeout_secs);

        while start.elapsed() < timeout {
            let mut cmd = Command::new(cli_binary);
            cmd.arg("status");

            // Configure transport mode
            if let Some(port) = self.port {
                cmd.env("DD_PM_USE_TCP", "1");
                cmd.env("DD_PM_DAEMON_PORT", port.to_string());
            } else if let Some(socket) = &self.socket {
                cmd.env("DD_PM_GRPC_SOCKET", socket);
            }

            if let Ok(output) = cmd.output() {
                if output.status.success() {
                    return true;
                }
            }

            thread::sleep(Duration::from_millis(100));
        }

        false
    }
}

/// Legacy function for backward compatibility
/// Wait for daemon to be healthy using health check
fn wait_for_daemon_health(port: u16, timeout_secs: u64) -> bool {
    DaemonHealthCheck::new()
        .port(port)
        .timeout(timeout_secs)
        .wait()
}

fn start_daemon_internal(mut cmd: Command, description: &str, _sleep_secs: u64) -> u16 {
    let port = get_unique_port();

    // Create temp files for capturing daemon logs
    let stdout_path = format!("/tmp/daemon-{}-{}.stdout.log", port, std::process::id());
    let stderr_path = format!("/tmp/daemon-{}-{}.stderr.log", port, std::process::id());

    let stdout_file =
        std::fs::File::create(&stdout_path).expect("Failed to create stdout log file");
    let stderr_file =
        std::fs::File::create(&stderr_path).expect("Failed to create stderr log file");

    // Set RUST_LOG if not already set (enables debug logging)
    if std::env::var("RUST_LOG").is_err() {
        cmd.env("RUST_LOG", "debug");
    }

    let daemon = cmd
        // Configure daemon via environment variables (no CLI args)
        .env("DD_PM_TRANSPORT_MODE", "tcp")
        .env("DD_PM_GRPC_PORT", port.to_string())
        .env("DD_PM_GRPC_SOCKET", unique_socket_path(port))
        .stdout(Stdio::from(stdout_file))
        .stderr(Stdio::from(stderr_file))
        .spawn()
        .expect("Failed to start daemon");

    let daemon_pid = daemon.id();

    // Store daemon PID, port, and log file paths in thread-local storage IMMEDIATELY
    // This ensures the panic hook can print logs even if daemon fails to start
    DAEMON_PID.with(|pid| *pid.borrow_mut() = Some(daemon_pid));
    TEST_PORT.with(|p| *p.borrow_mut() = Some(port));
    DAEMON_LOG_FILES
        .with(|logs| *logs.borrow_mut() = Some((stdout_path.clone(), stderr_path.clone())));
    TRANSPORT_MODE.with(|mode| *mode.borrow_mut() = Some(true)); // TCP mode

    // Wait for daemon to be healthy instead of fixed sleep
    if !wait_for_daemon_health(port, 10) {
        panic!(
            "Daemon failed to become healthy within 10 seconds (PID: {}, port: {})",
            daemon_pid, port
        );
    }

    println!("{} (PID: {}, port: {})", description, daemon_pid, port);
    println!("  Logs: stdout={}, stderr={}", stdout_path, stderr_path);

    // Keep daemon running (we kill it later with cleanup_daemon)
    std::mem::forget(daemon);

    port
}

// ============================================================================
// Modern Builder Pattern API (Recommended for new tests)
// ============================================================================

/// Builder for configuring daemon startup in tests
///
/// Provides a fluent interface for setting daemon environment variables.
/// Only non-default values need to be specified.
///
/// # Example
///
/// ```rust,ignore
/// // Start daemon with defaults (Unix socket)
/// let daemon = DaemonBuilder::new().build();
///
/// // Start daemon with custom configuration
/// let daemon = DaemonBuilder::new()
///     .transport_mode("tcp")
///     .grpc_port(55123)
///     .log_level("trace")
///     .config_file("/tmp/test.yaml")
///     .build();
/// ```
#[allow(dead_code)]
pub struct DaemonBuilder {
    transport_mode: Option<String>,
    grpc_port: Option<u16>,
    rest_port: Option<u16>,
    grpc_socket: Option<String>,
    rest_socket: Option<String>,
    enable_rest: Option<bool>,
    config_file: Option<String>,
    config_dir: Option<String>,
    log_level: Option<String>,
}

#[allow(dead_code)]
impl Default for DaemonBuilder {
    fn default() -> Self {
        Self::new()
    }
}

#[allow(dead_code)]
impl DaemonBuilder {
    /// Create a new daemon builder with default settings
    pub fn new() -> Self {
        Self {
            transport_mode: None,
            grpc_port: None,
            rest_port: None,
            grpc_socket: None,
            rest_socket: None,
            enable_rest: None,
            config_file: None,
            config_dir: None,
            log_level: None,
        }
    }

    /// Set transport mode: "unix" or "tcp"
    pub fn transport_mode(mut self, mode: &str) -> Self {
        self.transport_mode = Some(mode.to_string());
        self
    }

    /// Set gRPC port (TCP mode only)
    pub fn grpc_port(mut self, port: u16) -> Self {
        self.grpc_port = Some(port);
        self
    }

    /// Set REST port (TCP mode only)
    pub fn rest_port(mut self, port: u16) -> Self {
        self.rest_port = Some(port);
        self
    }

    /// Set gRPC Unix socket path
    pub fn grpc_socket(mut self, path: &str) -> Self {
        self.grpc_socket = Some(path.to_string());
        self
    }

    /// Set REST Unix socket path
    pub fn rest_socket(mut self, path: &str) -> Self {
        self.rest_socket = Some(path.to_string());
        self
    }

    /// Enable REST API
    pub fn enable_rest(mut self, enable: bool) -> Self {
        self.enable_rest = Some(enable);
        self
    }

    /// Set config file path
    pub fn config_file(mut self, path: &str) -> Self {
        self.config_file = Some(path.to_string());
        self
    }

    /// Set config directory path
    pub fn config_dir(mut self, path: &str) -> Self {
        self.config_dir = Some(path.to_string());
        self
    }

    /// Set log level (debug, info, warn, error, trace)
    pub fn log_level(mut self, level: &str) -> Self {
        self.log_level = Some(level.to_string());
        self
    }

    /// Build and start the daemon with configured settings
    ///
    /// Returns a guard that will automatically clean up the daemon when dropped.
    /// Integrates with the existing thread-local storage and automatic log printing on test failure.
    pub fn build(self) -> DaemonGuard {
        // If no transport mode specified, use defaults (Unix socket)
        // If TCP mode or specific port, use TCP
        let use_tcp = self.transport_mode.as_deref() == Some("tcp") || self.grpc_port.is_some();

        if use_tcp {
            // TCP mode: use the existing start_daemon_internal
            let mut cmd = Command::new(get_daemon_binary());

            // Apply config file/dir if specified
            if let Some(file) = self.config_file {
                cmd.env("DD_PM_CONFIG_FILE", file);
            }
            if let Some(dir) = self.config_dir {
                cmd.env("DD_PM_CONFIG_DIR", dir);
            }
            if let Some(level) = self.log_level {
                cmd.env("DD_PM_LOG_LEVEL", level);
            }
            if let Some(enable) = self.enable_rest {
                cmd.env("DD_PM_ENABLE_REST", enable.to_string());
            }

            start_daemon_internal(cmd, "Daemon started", 2);
        } else {
            // Unix socket mode: custom implementation
            let port = get_unique_port(); // Still need for log file naming
            let socket_path = self.grpc_socket.unwrap_or_else(|| unique_socket_path(port));

            // Create temp files for capturing daemon logs
            let stdout_path = format!("/tmp/daemon-{}-{}.stdout.log", port, std::process::id());
            let stderr_path = format!("/tmp/daemon-{}-{}.stderr.log", port, std::process::id());

            let stdout_file =
                std::fs::File::create(&stdout_path).expect("Failed to create stdout log file");
            let stderr_file =
                std::fs::File::create(&stderr_path).expect("Failed to create stderr log file");

            let mut cmd = Command::new(get_daemon_binary());

            // Set RUST_LOG if not already set
            if std::env::var("RUST_LOG").is_err() {
                cmd.env("RUST_LOG", "debug");
            }

            // Apply environment variables
            cmd.env("DD_PM_TRANSPORT_MODE", "unix");
            cmd.env("DD_PM_GRPC_SOCKET", &socket_path);

            if let Some(file) = self.config_file {
                cmd.env("DD_PM_CONFIG_FILE", file);
            }
            if let Some(dir) = self.config_dir {
                cmd.env("DD_PM_CONFIG_DIR", dir);
            }
            if let Some(level) = self.log_level {
                cmd.env("DD_PM_LOG_LEVEL", level);
            }
            if let Some(enable) = self.enable_rest {
                cmd.env("DD_PM_ENABLE_REST", enable.to_string());
            }

            let daemon = cmd
                .stdout(Stdio::from(stdout_file))
                .stderr(Stdio::from(stderr_file))
                .spawn()
                .expect("Failed to start daemon");

            let daemon_pid = daemon.id();

            // Store in thread-local storage
            DAEMON_PID.with(|pid| *pid.borrow_mut() = Some(daemon_pid));
            TEST_PORT.with(|p| *p.borrow_mut() = Some(port));
            DAEMON_LOG_FILES
                .with(|logs| *logs.borrow_mut() = Some((stdout_path.clone(), stderr_path.clone())));
            TRANSPORT_MODE.with(|mode| *mode.borrow_mut() = Some(false)); // Unix socket mode

            // Wait for daemon to be healthy
            if !DaemonHealthCheck::new()
                .socket(&socket_path)
                .timeout(10)
                .wait()
            {
                panic!(
                    "Daemon failed to become healthy within 10 seconds (PID: {}, socket: {})",
                    daemon_pid, socket_path
                );
            }

            println!(
                "Daemon started (PID: {}, socket: {})",
                daemon_pid, socket_path
            );
            println!("  Logs: stdout={}, stderr={}", stdout_path, stderr_path);

            // Keep daemon running
            std::mem::forget(daemon);
        }

        DaemonGuard { _private: () }
    }
}

/// Builder for configuring CLI commands in tests
///
/// Provides a fluent interface for running CLI commands with different
/// connection modes (TCP port, Unix socket, or defaults).
///
/// # Example
///
/// ```rust,ignore
/// // Connect via TCP port
/// let (stdout, stderr, code) = CliBuilder::new()
///     .port(50051)
///     .run(&["list"]);
///
/// // Connect via Unix socket
/// let (stdout, stderr, code) = CliBuilder::new()
///     .socket("/tmp/test.sock")
///     .run(&["list"]);
///
/// // Use defaults
/// let (stdout, stderr, code) = CliBuilder::new()
///     .run(&["list"]);
/// ```
#[allow(dead_code)]
pub struct CliBuilder {
    port: Option<u16>,
    socket: Option<String>,
}

#[allow(dead_code)]
impl Default for CliBuilder {
    fn default() -> Self {
        Self::new()
    }
}

#[allow(dead_code)]
impl CliBuilder {
    /// Create a new CLI builder
    pub fn new() -> Self {
        Self {
            port: None,
            socket: None,
        }
    }

    /// Connect via TCP port
    pub fn port(mut self, port: u16) -> Self {
        self.port = Some(port);
        self.socket = None; // Clear socket if port is set
        self
    }

    /// Connect via Unix socket
    pub fn socket(mut self, socket: &str) -> Self {
        self.socket = Some(socket.to_string());
        self.port = None; // Clear port if socket is set
        self
    }

    /// Use TCP mode (for backward compatibility)
    pub fn use_tcp(mut self) -> Self {
        // This is a no-op if port is already set
        // Otherwise, it will use the default TCP port
        if self.port.is_none() {
            self.port = Some(50051); // Default gRPC port
        }
        self
    }

    /// Run a CLI command with the configured connection
    ///
    /// Returns (stdout, stderr, exit_code)
    pub fn run(self, args: &[&str]) -> (String, String, i32) {
        let mut cmd = Command::new(get_cli_binary());

        // Add command arguments
        for arg in args {
            cmd.arg(arg);
        }

        // Configure connection mode
        if let Some(port) = self.port {
            cmd.env("DD_PM_USE_TCP", "1");
            cmd.env("DD_PM_DAEMON_PORT", port.to_string());
        } else if let Some(socket) = self.socket {
            cmd.env("DD_PM_GRPC_SOCKET", socket);
        } else {
            // Auto-detect from thread-local storage
            // Check if daemon is using TCP or Unix socket mode
            let is_tcp = TRANSPORT_MODE.with(|mode| mode.borrow().unwrap_or(false));
            if is_tcp {
                let port = TEST_PORT.with(|p| p.borrow().unwrap_or(50051));
                cmd.env("DD_PM_USE_TCP", "1");
                cmd.env("DD_PM_DAEMON_PORT", port.to_string());
            } else {
                // Unix socket mode - use temp socket path
                let port = TEST_PORT.with(|p| p.borrow().unwrap_or(50000));
                let socket = unique_socket_path(port);
                cmd.env("DD_PM_GRPC_SOCKET", socket);
            }
        }

        let output = cmd.output().expect("Failed to execute CLI");

        let stdout = String::from_utf8_lossy(&output.stdout).to_string();
        let stderr = String::from_utf8_lossy(&output.stderr).to_string();
        let code = output.status.code().unwrap_or(-1);

        (stdout, stderr, code)
    }
}

// ============================================================================
// Legacy API (Kept for backward compatibility)
// ============================================================================

/// Setup and start the daemon for isolated tests (with unique port)
/// Returns an RAII guard that ensures cleanup on drop (even on panic)
///
/// # Example
/// ```rust,ignore
/// #[test]
/// fn my_test() {
///     let _daemon = setup_daemon();
///     // Test code here - daemon will be cleaned up even if panic occurs
/// }
/// ```
#[allow(dead_code)]
pub fn setup_daemon() -> DaemonGuard {
    let cmd = Command::new(get_daemon_binary());
    start_daemon_internal(cmd, "Daemon started", 2);
    DaemonGuard { _private: () }
}

/// Setup and start the daemon with a config file (with unique port)
/// Returns an RAII guard that ensures cleanup on drop (even on panic)
#[allow(dead_code)]
pub fn setup_daemon_with_config_file(config_path: &str) -> DaemonGuard {
    let mut cmd = Command::new(get_daemon_binary());
    cmd.env("DD_PM_CONFIG_FILE", config_path);
    let port = start_daemon_internal(
        cmd,
        &format!("Daemon started with config file: {}", config_path),
        2,
    );
    println!("Daemon port: {}", port);
    DaemonGuard { _private: () }
}

/// Setup and start the daemon with a config directory (with unique port)
/// Returns an RAII guard that ensures cleanup on drop (even on panic)
#[allow(dead_code)]
pub fn setup_daemon_with_config_dir(config_dir: &str) -> DaemonGuard {
    let mut cmd = Command::new(get_daemon_binary());
    cmd.env("DD_PM_CONFIG_DIR", config_dir);
    let port = start_daemon_internal(
        cmd,
        &format!("Daemon started with config dir: {}", config_dir),
        2,
    );
    println!("Daemon port: {}", port);
    DaemonGuard { _private: () }
}

/// Setup and start the daemon with config via environment variable (with unique port)
/// Returns an RAII guard that ensures cleanup on drop (even on panic)
#[allow(dead_code)]
pub fn setup_daemon_with_config_env(config_path: &str) -> DaemonGuard {
    let mut cmd = Command::new(get_daemon_binary());
    cmd.env("DD_PM_CONFIG_PATH", config_path);
    let port = start_daemon_internal(
        cmd,
        &format!("Daemon started with config env: {}", config_path),
        2,
    );
    println!("Daemon port: {}", port);
    DaemonGuard { _private: () }
}

/// Kill only THIS test's daemon (not all daemons)
/// If PRINT_DAEMON_LOGS=1 is set, daemon logs will be printed before cleanup
#[allow(dead_code)]
pub fn cleanup_daemon() {
    // Print logs if requested (useful for CI debugging)
    print_daemon_logs_if_enabled();

    // Clean up all processes in the engine before killing daemon
    // This prevents global state pollution between tests
    let port = TEST_PORT.with(|p| p.borrow().unwrap_or(50051));
    let list_output = Command::new(get_cli_binary())
        .args(["list"])
        .env("DD_PM_USE_TCP", "1")
        .env("DD_PM_DAEMON_PORT", port.to_string())
        .output();

    if let Ok(output) = list_output {
        let stdout = String::from_utf8_lossy(&output.stdout);
        // Parse process IDs from list output and delete them
        for line in stdout.lines().skip(2) {
            // Skip header lines
            let parts: Vec<&str> = line.split_whitespace().collect();
            if parts.len() >= 2 {
                let id = parts[1]; // ID is second column
                if id.len() == 36 {
                    // UUID format check
                    let _ = Command::new(get_cli_binary())
                        .args(["delete", id, "--force"])
                        .env("DD_PM_USE_TCP", "1")
                        .env("DD_PM_DAEMON_PORT", port.to_string())
                        .output();
                }
            }
        }
    }

    // Kill only the daemon started by this test thread
    DAEMON_PID.with(|pid| {
        if let Some(daemon_pid) = *pid.borrow() {
            let _ = Command::new("kill")
                .args(["-9", &daemon_pid.to_string()])
                .output();
            // Wait for OS to release ports and clean up resources after SIGKILL
            // For TCP mode: 3 seconds is needed for sequential test execution to avoid
            // port reuse issues due to TIME_WAIT state.
            // For Unix socket mode: minimal wait is sufficient since sockets are file-based.
            TRANSPORT_MODE.with(|mode| {
                match *mode.borrow() {
                    Some(true) => {
                        // TCP mode - need to wait for TIME_WAIT
                        thread::sleep(Duration::from_secs(3));
                    }
                    Some(false) => {
                        // Unix socket mode - just a brief wait for cleanup
                        thread::sleep(Duration::from_millis(100));
                    }
                    None => {
                        // Unknown mode - be conservative and wait
                        thread::sleep(Duration::from_secs(3));
                    }
                }
            });
        }
    });

    // Clear the cached port so the next test on this thread gets a fresh port
    // This prevents "Address already in use" when multiple sequential tests
    // run on the same thread (e.g., with --test-threads=1)
    TEST_PORT.with(|port| {
        *port.borrow_mut() = None;
    });
    DAEMON_PID.with(|pid| {
        *pid.borrow_mut() = None;
    });
    DAEMON_LOG_FILES.with(|logs| {
        *logs.borrow_mut() = None;
    });
    TRANSPORT_MODE.with(|mode| {
        *mode.borrow_mut() = None;
    });
}

/// Print daemon logs if PRINT_DAEMON_LOGS environment variable is set
#[allow(dead_code)]
pub fn print_daemon_logs_if_enabled() {
    if std::env::var("PRINT_DAEMON_LOGS").is_ok() {
        print_daemon_logs();
    }
}

/// Execute CLI command and return only stdout (most common case)
#[allow(dead_code)]
pub fn run_cli(args: &[&str]) -> String {
    let (stdout, _, _) = run_cli_full(args);
    stdout
}

/// Execute CLI command and return (stdout, stderr, exit_code)
/// Automatically uses the port assigned to the current test thread
pub fn run_cli_full(args: &[&str]) -> (String, String, i32) {
    let port = TEST_PORT.with(|p| p.borrow().unwrap_or(DEFAULT_PORT));

    let output = Command::new(get_cli_binary())
        .args(args)
        .env("DD_PM_USE_TCP", "1")
        .env("DD_PM_DAEMON_PORT", port.to_string())
        .output()
        .expect("Failed to execute CLI");

    let stdout = String::from_utf8_lossy(&output.stdout).to_string();
    let stderr = String::from_utf8_lossy(&output.stderr).to_string();
    let exit_code = output.status.code().unwrap_or(-1);

    (stdout, stderr, exit_code)
}

/// Extract process ID from CLI output
#[allow(dead_code)]
pub fn extract_process_id(output: &str) -> Option<&str> {
    output
        .lines()
        .find(|l| l.contains("ID:"))
        .and_then(|l| l.split_whitespace().nth(1))
}

/// Generate a unique process name using daemon PID and timestamp to avoid test isolation issues
/// Format: e2e-d{daemon_pid}-{timestamp}
/// This helps with troubleshooting by showing which daemon/test created the process
#[allow(dead_code)]
pub fn unique_process_name() -> String {
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::time::{SystemTime, UNIX_EPOCH};

    static COUNTER: AtomicU64 = AtomicU64::new(0);

    // Get thread ID as a unique identifier
    let thread_id = std::thread::current().id();
    let thread_id_str = format!("{:?}", thread_id)
        .chars()
        .filter(|c| c.is_numeric())
        .collect::<String>();
    let thread_id_num: u64 = thread_id_str.parse().unwrap_or(0);

    // Use high-precision timestamp
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_nanos();

    // Add counter for additional uniqueness within same timestamp
    let counter = COUNTER.fetch_add(1, Ordering::SeqCst);

    // Combine: thread_id + timestamp + counter
    let unique_num = (thread_id_num
        .wrapping_mul(1000000)
        .wrapping_add((timestamp % 1000000) as u64)
        .wrapping_add(counter))
        % 1000000;

    format!("e2e-t{}-{}", thread_id_num % 10000, unique_num)
}

/// Create a process and return its ID
/// The create operation is synchronous - process is in "created" state immediately
#[allow(dead_code)]
pub fn create_process(name: &str, command: &str, args: &[&str]) -> String {
    let mut cmd_args = vec!["create", name, command];
    cmd_args.extend(args);

    let (stdout, stderr, exit_code) = run_cli_full(&cmd_args);
    assert_eq!(exit_code, 0, "Create failed: {}", stderr);

    extract_process_id(&stdout)
        .expect("Failed to extract process ID")
        .to_string()
}

/// Start a process by ID and wait for it to reach running state (with 5s timeout)
/// Panics if the process doesn't start or reach running state
/// For custom timeouts, use start_process_with_timeout
#[allow(dead_code)]
pub fn start_process(id: &str) {
    start_process_with_timeout(id, 5);
}

/// Start a process and wait for it to reach running state
/// Returns Ok(()) if successful, Err with message if timeout or start fails
#[allow(dead_code)]
pub fn try_start_process_with_timeout(id: &str, timeout_secs: u64) -> Result<(), String> {
    let (_, stderr, exit_code) = run_cli_full(&["start", id]);
    if exit_code != 0 {
        return Err(format!("Start failed: {}", stderr));
    }

    if !wait_for_state(id, "running", timeout_secs) {
        // Get actual current state for troubleshooting
        let actual_state = get_process_state_by_id(id);

        return Err(format!(
            "Process '{}' did not reach running state within {} seconds. Current state: {}",
            id, timeout_secs, actual_state
        ));
    }

    Ok(())
}

/// Start a process and wait for it to reach running state
/// Panics if the process doesn't reach running state within timeout_secs
/// Convenience wrapper around try_start_process_with_timeout
#[allow(dead_code)]
pub fn start_process_with_timeout(id: &str, timeout_secs: u64) {
    try_start_process_with_timeout(id, timeout_secs)
        .expect("Failed to start process and wait for running state");
}

/// Stop a process by ID and wait for it to reach stopped state (with 5s timeout)
/// Panics if the process doesn't stop or reach stopped state
/// For custom timeouts, use stop_process_with_timeout
#[allow(dead_code)]
pub fn stop_process(id: &str) {
    stop_process_with_timeout(id, 5);
}

/// Stop a process and wait for it to reach stopped state
/// Returns Ok(()) if successful, Err with message if timeout or stop fails
#[allow(dead_code)]
pub fn try_stop_process_with_timeout(id: &str, timeout_secs: u64) -> Result<(), String> {
    let (_, stderr, exit_code) = run_cli_full(&["stop", id]);
    if exit_code != 0 {
        return Err(format!("Stop failed: {}", stderr));
    }

    if !wait_for_state(id, "stopped", timeout_secs) {
        return Err(format!(
            "Process '{}' did not reach stopped state within {} seconds",
            id, timeout_secs
        ));
    }

    Ok(())
}

/// Stop a process and wait for it to reach stopped state
/// Panics if the process doesn't reach stopped state within timeout_secs
/// Convenience wrapper around try_stop_process_with_timeout
#[allow(dead_code)]
pub fn stop_process_with_timeout(id: &str, timeout_secs: u64) {
    try_stop_process_with_timeout(id, timeout_secs)
        .expect("Failed to stop process and wait for stopped state");
}

/// Delete a process by ID (with force)
#[allow(dead_code)]
pub fn delete_process(id: &str) {
    let _ = run_cli_full(&["delete", id, "--force"]);
}

/// Setup temp directory for tests
#[allow(dead_code)]
pub fn setup_temp_dir() -> tempfile::TempDir {
    tempfile::TempDir::new().expect("Failed to create temp directory")
}

/// Deprecated: Use setup_daemon_with_config_file() for normal tests
/// Setup daemon with explicit port and config, return process handle for manual cleanup
/// Only use this for infrastructure tests that need manual port/process control
#[allow(dead_code)]
#[deprecated(
    since = "0.1.0",
    note = "Use setup_daemon_with_config_file() instead. Only use this for daemon infrastructure tests."
)]
pub fn setup_daemon_with_port_and_config(port: u16, config_path: &str) -> std::process::Child {
    let mut cmd = Command::new(get_daemon_binary());
    // Configure daemon via environment variables (no CLI args)
    cmd.env("DD_PM_CONFIG_FILE", config_path)
        .env("DD_PM_TRANSPORT_MODE", "tcp")
        .env("DD_PM_GRPC_PORT", port.to_string())
        .env("DD_PM_GRPC_SOCKET", unique_socket_path(port))
        .stdout(Stdio::null())
        .stderr(Stdio::null());

    let daemon = cmd.spawn().expect("Failed to start daemon");
    let daemon_pid = daemon.id();

    println!(
        "Daemon started with config: {} (PID: {}, port: {})",
        config_path, daemon_pid, port
    );

    daemon
}

/// Cleanup daemon process handle
#[allow(dead_code)]
pub fn cleanup_daemon_proc(mut proc: std::process::Child) {
    let _ = proc.kill();
    let _ = proc.wait();
    thread::sleep(Duration::from_millis(500));
}

/// Run CLI command with explicit port
#[allow(dead_code)]
pub fn run_cli_port(args: &[&str], port: u16) -> (String, String, i32) {
    let output = Command::new(get_cli_binary())
        .args(args)
        .env("DD_PM_USE_TCP", "1")
        .env("DD_PM_DAEMON_PORT", port.to_string())
        .output()
        .expect("Failed to execute CLI");

    let stdout = String::from_utf8_lossy(&output.stdout).to_string();
    let stderr = String::from_utf8_lossy(&output.stderr).to_string();
    let exit_code = output.status.code().unwrap_or(-1);

    (stdout, stderr, exit_code)
}

/// Extract port from a daemon Child process
/// Returns the port that was used to start the daemon
#[allow(dead_code)]
pub fn extract_port_from_child(_proc: &std::process::Child) -> u16 {
    // The daemon was started with setup_daemon_with_port_and_config
    // which takes an explicit port. For socket tests, we need to track this.
    // Since we can't easily extract it from the Child, we use a convention:
    // socket tests should use get_socket_test_port() and pass that to setup

    // As a workaround, check if there's a port file we can read
    // For now, return the standard daemon port for backwards compatibility
    // Tests that need specific ports should manage them explicitly
    50051
}

/// Get a unique port for socket tests
/// Uses a separate range from daemon gRPC ports to avoid conflicts
/// Daemon uses: 50000-59999
/// Socket tests use: 60000+
#[allow(dead_code)]
pub fn get_socket_test_port() -> u16 {
    static SOCKET_PORT_COUNTER: std::sync::atomic::AtomicU16 = std::sync::atomic::AtomicU16::new(0);
    let offset = SOCKET_PORT_COUNTER.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
    60000 + offset
}

// ============================================================================
// Additional Test Helpers
// ============================================================================

/// Internal helper: Check process state by matching a specific column
/// column_index: 0 for name, 1 for ID
/// Returns true if process found and state matches, false otherwise
fn check_process_state_internal(
    identifier: &str,
    expected_state: &str,
    column_index: usize,
) -> bool {
    let output = run_cli(&["list"]);
    for line in output.lines() {
        let cols: Vec<&str> = line.split_whitespace().collect();
        // STATE column is at index 3 (NAME ID PID STATE ...)
        if cols.len() >= 4 && cols.get(column_index) == Some(&identifier) {
            return cols[3] == expected_state;
        }
    }
    false
}

/// Get the current state of a process by ID (returns state string or "not found")
fn get_process_state_by_id(process_id: &str) -> String {
    let output = run_cli(&["list"]);
    for line in output.lines() {
        let cols: Vec<&str> = line.split_whitespace().collect();
        // STATE column is at index 3 (NAME ID PID STATE ...)
        if cols.len() >= 4 && cols.get(1) == Some(&process_id) {
            return cols[3].to_string();
        }
    }
    "not found".to_string()
}

/// Check if a process is in a specific state by ID (returns bool, doesn't panic)
#[allow(dead_code)]
pub fn check_process_state_by_id(process_id: &str, expected_state: &str) -> bool {
    check_process_state_internal(process_id, expected_state, 1)
}

/// Check if a process is in a specific state by name (returns bool, doesn't panic)
#[allow(dead_code)]
pub fn check_process_state_by_name(process_name: &str, expected_state: &str) -> bool {
    check_process_state_internal(process_name, expected_state, 0)
}

/// Wait for a process to reach a specific state (with timeout)
/// Works with both process IDs (UUID) and process names
/// Automatically detects if identifier is a UUID or name
/// Returns true if state was reached, false if timeout
#[allow(dead_code)]
pub fn wait_for_state(identifier: &str, expected_state: &str, timeout_secs: u64) -> bool {
    let start = std::time::Instant::now();
    let timeout = Duration::from_secs(timeout_secs);

    // Check if identifier looks like a UUID (8-4-4-4-12 format, 36 chars total)
    // UUIDs are exactly 36 characters and have 4 dashes in specific positions
    let is_uuid = identifier.len() == 36 && identifier.chars().filter(|c| *c == '-').count() == 4;

    while start.elapsed() < timeout {
        let state_matches = if is_uuid {
            check_process_state_by_id(identifier, expected_state)
        } else {
            check_process_state_by_name(identifier, expected_state)
        };

        if state_matches {
            return true;
        }
        thread::sleep(Duration::from_millis(100));
    }
    false
}

/// Create, start, and wait for a process to be running
/// Returns the process ID
/// Panics if the process doesn't reach running state within timeout_secs (default: 5)
#[allow(dead_code)]
pub fn create_and_start_process(name: &str, command: &str, args: &[&str]) -> String {
    create_and_start_process_with_timeout(name, command, args, 5)
}

/// Create, start, and wait for a process to be running with custom timeout
/// Returns the process ID
/// Panics if the process doesn't reach running state within timeout_secs
#[allow(dead_code)]
pub fn create_and_start_process_with_timeout(
    name: &str,
    command: &str,
    args: &[&str],
    timeout_secs: u64,
) -> String {
    let id = create_process(name, command, args);
    start_process_with_timeout(&id, timeout_secs);
    id
}

/// Verify a process is in a specific state by checking the list output by name
/// Returns true if the process is in the expected state, false otherwise
/// Matches exact process name (column 0) and state (column 2)
#[allow(dead_code)]
pub fn assert_process_state_by_name(process_name: &str, expected_state: &str) -> bool {
    check_process_state_by_name(process_name, expected_state)
}

/// Verify a process is in a specific state by checking the list output by ID
/// Returns true if the process is in the expected state, false otherwise
/// Matches exact process ID (column 1) and state (column 2)
#[allow(dead_code)]
pub fn assert_process_state_by_id(process_id: &str, expected_state: &str) -> bool {
    check_process_state_by_id(process_id, expected_state)
}

/// Check if a process exists in the list by name
/// Returns true if the process appears in the list output
/// Matches exact process name (column 0)
#[allow(dead_code)]
pub fn process_exists_by_name(process_name: &str) -> bool {
    let list_output = run_cli(&["list"]);
    for line in list_output.lines() {
        let columns: Vec<&str> = line.split_whitespace().collect();
        if !columns.is_empty() && columns[0] == process_name {
            return true;
        }
    }
    false
}

/// Check if a process exists in the list by ID
/// Returns true if the process appears in the list output
/// Matches exact process ID (column 1)
#[allow(dead_code)]
pub fn process_exists_by_id(process_id: &str) -> bool {
    let list_output = run_cli(&["list"]);
    for line in list_output.lines() {
        let columns: Vec<&str> = line.split_whitespace().collect();
        if columns.len() >= 2 && columns[1] == process_id {
            return true;
        }
    }
    false
}

/// Print daemon logs to stdout (useful for debugging CI failures)
///
/// Call this in your test when you want to see daemon logs:
/// ```rust,ignore
/// if some_condition_failed {
///     print_daemon_logs();
///     panic!("Test failed");
/// }
/// ```
#[allow(dead_code)]
pub fn print_daemon_logs() {
    DAEMON_LOG_FILES.with(|logs| {
        if let Some((stdout_path, stderr_path)) = logs.borrow().as_ref() {
            println!("\n========== DAEMON STDOUT ==========");
            if let Ok(stdout_content) = std::fs::read_to_string(stdout_path) {
                println!("{}", stdout_content);
            } else {
                println!("Failed to read stdout log: {}", stdout_path);
            }

            println!("\n========== DAEMON STDERR ==========");
            if let Ok(stderr_content) = std::fs::read_to_string(stderr_path) {
                println!("{}", stderr_content);
            } else {
                println!("Failed to read stderr log: {}", stderr_path);
            }
            println!("===================================\n");
        } else {
            println!("No daemon logs available (daemon not started?)");
        }
    });
}

/// Print last N lines of daemon logs (useful for debugging without overwhelming output)
#[allow(dead_code)]
pub fn print_daemon_logs_tail(lines: usize) {
    DAEMON_LOG_FILES.with(|logs| {
        if let Some((stdout_path, stderr_path)) = logs.borrow().as_ref() {
            println!(
                "\n========== DAEMON STDOUT (last {} lines) ==========",
                lines
            );
            if let Ok(stdout_content) = std::fs::read_to_string(stdout_path) {
                let tail: Vec<&str> = stdout_content.lines().rev().take(lines).collect();
                for line in tail.iter().rev() {
                    println!("{}", line);
                }
            } else {
                println!("Failed to read stdout log: {}", stdout_path);
            }

            println!(
                "\n========== DAEMON STDERR (last {} lines) ==========",
                lines
            );
            if let Ok(stderr_content) = std::fs::read_to_string(stderr_path) {
                let tail: Vec<&str> = stderr_content.lines().rev().take(lines).collect();
                for line in tail.iter().rev() {
                    println!("{}", line);
                }
            } else {
                println!("Failed to read stderr log: {}", stderr_path);
            }
            println!("===================================\n");
        } else {
            println!("No daemon logs available (daemon not started?)");
        }
    });
}

/// Verify a process has a specific exit code by checking the list output
/// Returns true if the process exit code matches the expected value
/// Matches exact process ID (column 1) and checks EXIT column (last column)
/// Note: Exit code 0 is displayed as "-" in the list output
#[allow(dead_code)]
pub fn assert_process_exit_code_by_id(process_id: &str, expected_exit_code: i32) -> bool {
    let list_output = run_cli(&["list"]);
    for line in list_output.lines() {
        // Split line into columns - EXIT is always the last column
        let columns: Vec<&str> = line.split_whitespace().collect();
        // Check if this is the right process (ID is column 1)
        if columns.len() >= 2 && columns[1] == process_id {
            // EXIT is the last column
            if let Some(exit_str) = columns.last() {
                // "-" means exit code 0 or no signal
                if *exit_str == "-" {
                    return expected_exit_code == 0;
                }
                // Try to parse as integer (exit code)
                if let Ok(exit_code) = exit_str.parse::<i32>() {
                    return exit_code == expected_exit_code;
                }
                // Otherwise it's a signal name (e.g., "SIGTERM (15)")
                // For now, just return false for non-zero expected codes
                return false;
            }
        }
    }
    false
}
