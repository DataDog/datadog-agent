//! Domain Constants
//!
//! Common constants used throughout the domain layer

/// Exit code indicating successful process termination
pub const SUCCESS_EXIT_CODE: i32 = 0;

/// Default restart delay in seconds
pub const DEFAULT_RESTART_DELAY_SEC: u64 = 1;

/// Default maximum restart delay in seconds
pub const DEFAULT_RESTART_MAX_DELAY_SEC: u64 = 60;

/// Default start timeout in seconds
pub const DEFAULT_START_TIMEOUT_SEC: u64 = 90;

/// Default stop timeout in seconds
pub const DEFAULT_STOP_TIMEOUT_SEC: u64 = 90;

/// Default start limit burst (number of restarts allowed in interval)
pub const DEFAULT_START_LIMIT_BURST: u32 = 5;

/// Default start limit interval in seconds
pub const DEFAULT_START_LIMIT_INTERVAL_SEC: u64 = 10;

/// Exponential backoff base for restart delays
pub const RESTART_BACKOFF_BASE: u32 = 2;

/// Default runtime success threshold in seconds
/// 0 means disabled - use health check success to reset failures instead
/// If > 0, process is considered "successful" after running this long
pub const DEFAULT_RUNTIME_SUCCESS_SEC: u64 = 0;

/// Default CPU limit in millicores (1 core = 1000 millicores)
pub const DEFAULT_CPU_LIMIT_MILLIS: u64 = 1000;

/// Default memory limit in bytes (512 MB)
pub const DEFAULT_MEMORY_LIMIT_BYTES: u64 = 512 * BYTES_PER_MB;

/// Default PIDs limit
pub const DEFAULT_PIDS_LIMIT: u32 = 100;

/// Minimum valid CPU limit in millicores
pub const MIN_CPU_LIMIT_MILLIS: u64 = 1;

/// Minimum valid memory limit in bytes (1 MB)
pub const MIN_MEMORY_LIMIT_BYTES: u64 = BYTES_PER_MB;

/// Minimum valid PIDs limit
pub const MIN_PIDS_LIMIT: u32 = 1;

/// Memory unit constants
pub const BYTES_PER_KB: u64 = 1024;
pub const BYTES_PER_MB: u64 = 1024 * 1024;
pub const BYTES_PER_GB: u64 = 1024 * 1024 * 1024;
pub const BYTES_PER_TB: u64 = 1024 * 1024 * 1024 * 1024;
