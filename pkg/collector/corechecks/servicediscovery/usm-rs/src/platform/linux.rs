#[cfg(target_os = "linux")]
use std::path::PathBuf;

/// Get working directory from process ID on Linux
#[cfg(target_os = "linux")]
pub fn get_working_directory_from_pid(pid: u32) -> Result<PathBuf, std::io::Error> {
    let cwd_path = format!("/proc/{}/cwd", pid);
    let target = std::fs::read_link(&cwd_path)?;
    Ok(target)
}

#[cfg(not(target_os = "linux"))]
pub fn get_working_directory_from_pid(_pid: u32) -> Result<std::path::PathBuf, std::io::Error> {
    Err(std::io::Error::new(
        std::io::ErrorKind::Unsupported,
        "get_working_directory_from_pid not supported on this platform"
    ))
}