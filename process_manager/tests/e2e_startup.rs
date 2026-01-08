//! E2E tests for daemon startup and initialization
//!
//! Tests the daemon's ability to start correctly under various configurations
//! and conditions. This is the first test suite users encounter when setting
//! up the process manager.
//!
//! ## Test Organization
//!
//! Each test follows the AAA (Arrange-Act-Assert) pattern:
//! - **Arrange**: Set up daemon configuration using DaemonBuilder
//! - **Act**: Start daemon and perform operations
//! - **Assert**: Verify expected behavior
//!
//! ## Daemon Builder Pattern
//!
//! Tests use `DaemonBuilder` to configure non-default options:
//!
//! ```rust,ignore
//! let _daemon = DaemonBuilder::new()
//!     .transport_mode("tcp")
//!     .grpc_port(55000)
//!     .log_level("debug")
//!     .build();
//! ```

mod common;
use common::{CliBuilder, DaemonBuilder};

// ============================================================================
// TEST CASE 1: Basic Startup - Daemon starts with defaults
// ============================================================================

#[test]
fn test_daemon_starts_with_defaults() {
    // ARRANGE: Start daemon with no configuration (pure defaults)
    let _daemon = DaemonBuilder::new().build();

    // ACT: Connect using CLI defaults
    let (stdout, stderr, code) = CliBuilder::new().run(&["list"]);

    // ASSERT: Verify functionality
    assert_eq!(code, 0, "CLI command failed: {}", stderr);
    assert!(
        stdout.contains("No processes"),
        "Expected empty process list"
    );

    // ASSERT: Verify daemon is ready
    let logs = _daemon.stdout_logs();
    assert!(
        logs.contains("Daemon ready. All services started"),
        "Daemon should log ready message"
    );
}

// ============================================================================
// Additional test cases will be added here one by one for review
// ============================================================================
