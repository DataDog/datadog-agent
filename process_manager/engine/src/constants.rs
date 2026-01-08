//! Application-wide constants and default values
//!
//! Centralizes magic numbers and default configurations for better maintainability

/// Event system configuration
pub mod events {
    /// Number of worker threads in the event handler thread pool
    pub const THREAD_POOL_SIZE: usize = 4;
}

/// Process restart and lifecycle defaults (systemd-compatible)
pub mod process {
    /// Default delay before restart (seconds)
    pub const DEFAULT_RESTART_SEC: u64 = 1;

    /// Maximum delay between restarts (seconds, for exponential backoff)
    pub const DEFAULT_RESTART_MAX_DELAY: u64 = 60;

    /// Maximum number of restarts within the interval
    pub const DEFAULT_START_LIMIT_BURST: u32 = 5;

    /// Time window for start limit tracking (seconds, 5 minutes)
    pub const DEFAULT_START_LIMIT_INTERVAL: u64 = 300;

    /// Default timeout for graceful stop (seconds, systemd default)
    pub const DEFAULT_TIMEOUT_STOP_SEC: u64 = 90;

    /// Default signal for process termination
    pub const DEFAULT_KILL_SIGNAL: &str = "SIGTERM";

    /// Default success exit code
    pub const DEFAULT_SUCCESS_EXIT_CODE: i32 = 0;
}

/// Health check defaults
pub mod health_check {
    /// Default interval between health checks (seconds)
    pub const DEFAULT_INTERVAL: u64 = 30;

    /// Default timeout for health check operations (seconds)
    pub const DEFAULT_TIMEOUT: u64 = 5;

    /// Default number of retries before marking unhealthy
    pub const DEFAULT_RETRIES: u32 = 3;

    /// Default HTTP method for HTTP health checks
    pub const DEFAULT_HTTP_METHOD: &str = "GET";

    /// Default expected HTTP status code
    pub const DEFAULT_HTTP_STATUS: u16 = 200;
}

/// Test infrastructure constants
pub mod test {
    /// Base port for test daemon instances
    pub const BASE_PORT: u16 = 50000;

    /// Port range size for test allocation
    pub const PORT_RANGE: u16 = 10000;

    /// Default port when no test port is assigned
    pub const DEFAULT_PORT: u16 = 50051;
}
