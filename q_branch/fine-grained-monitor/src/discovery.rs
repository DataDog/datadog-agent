//! Container discovery via cgroup filesystem scanning (REQ-FM-001)
//!
//! Discovers running containers by walking /sys/fs/cgroup/ and matching
//! kubepods patterns. No external dependencies (kubelet, CRI socket).

use std::path::PathBuf;

/// A discovered container with its cgroup path and PIDs
#[derive(Debug, Clone)]
pub struct Container {
    /// Container ID extracted from cgroup path
    pub id: String,
    /// Full path to the container's cgroup directory
    pub cgroup_path: PathBuf,
    /// PIDs belonging to this container (from cgroup.procs)
    pub pids: Vec<i32>,
}

/// Scan the cgroup filesystem for running containers
///
/// Walks /sys/fs/cgroup/ looking for patterns like:
/// - `cri-containerd-<id>.scope` (containerd)
/// - `crio-<id>.scope` (CRI-O)
/// - `docker-<id>.scope` (Docker)
pub fn scan_cgroups() -> Vec<Container> {
    // TODO: Implement REQ-FM-001
    // 1. Walk /sys/fs/cgroup/kubepods.slice/ recursively
    // 2. Match container scope directories
    // 3. Extract container ID from directory name
    // 4. Read PIDs from cgroup.procs
    Vec::new()
}
