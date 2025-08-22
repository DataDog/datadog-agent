#[cfg(not(target_os = "linux"))]
use std::path::PathBuf;

/// Stub implementation for non-Linux platforms
#[cfg(not(target_os = "linux"))]
pub fn get_working_directory_from_pid(_pid: u32) -> Result<PathBuf, std::io::Error> {
    Err(std::io::Error::new(
        std::io::ErrorKind::Unsupported,
        "Working directory from PID not supported on this platform",
    ))
}