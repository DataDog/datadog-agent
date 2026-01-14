//! Container discovery via cgroup filesystem scanning (REQ-FM-001)
//!
//! Discovers running containers by walking /sys/fs/cgroup/ and matching
//! kubepods patterns. No external dependencies (kubelet, CRI socket).

use std::collections::HashMap;
use std::fs;
use std::io::{self, BufRead};
use std::path::{Path, PathBuf};

/// Default cgroup root path
const CGROUP_ROOT: &str = "/sys/fs/cgroup";

/// A discovered container with its cgroup path and PIDs
#[derive(Debug, Clone)]
#[cfg_attr(not(target_os = "linux"), allow(dead_code))]
pub struct Container {
    /// Container ID extracted from cgroup path
    pub id: String,
    /// Full path to the container's cgroup directory
    pub cgroup_path: PathBuf,
    /// PIDs belonging to this container (from cgroup.procs)
    pub pids: Vec<i32>,
    /// Pod UID extracted from parent cgroup path (if available)
    pub pod_uid: Option<String>,
    /// QoS class inferred from cgroup path
    pub qos_class: QosClass,
    // REQ-MV-016: Kubernetes metadata from API (populated by kubernetes.rs)
    /// Pod name from Kubernetes API (e.g., "coredns-5dd5756b68-abc12")
    pub pod_name: Option<String>,
    /// Container name from Kubernetes API (e.g., "monitor", "viewer")
    pub container_name: Option<String>,
    /// Namespace from Kubernetes API (e.g., "kube-system")
    pub namespace: Option<String>,
    /// Pod labels from Kubernetes API
    pub labels: Option<HashMap<String, String>>,
}

/// Kubernetes QoS class
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum QosClass {
    Guaranteed,
    Burstable,
    BestEffort,
    Unknown,
}

impl QosClass {
    fn from_path(path: &Path) -> Self {
        let path_str = path.to_string_lossy();
        if path_str.contains("besteffort") {
            QosClass::BestEffort
        } else if path_str.contains("burstable") {
            QosClass::Burstable
        } else if path_str.contains("kubepods") {
            // Guaranteed pods are directly under kubepods (no qos subdirectory)
            QosClass::Guaranteed
        } else {
            QosClass::Unknown
        }
    }
}

/// Scan the cgroup filesystem for running containers
///
/// Walks /sys/fs/cgroup/ looking for patterns like:
/// - `cri-containerd-<id>.scope` (containerd)
/// - `crio-<id>.scope` (CRI-O)
/// - `docker-<id>.scope` (Docker)
///
/// Returns a list of discovered containers with their cgroup paths and PIDs.
pub fn scan_cgroups() -> Vec<Container> {
    scan_cgroups_at(Path::new(CGROUP_ROOT))
}

/// Scan for containers starting at the given cgroup root path
///
/// This is the testable version that accepts a custom root path.
pub fn scan_cgroups_at(cgroup_root: &Path) -> Vec<Container> {
    let mut containers = Vec::new();

    // Walk the cgroup tree recursively
    if let Err(e) = walk_cgroups(cgroup_root, &mut containers) {
        tracing::warn!(
            path = %cgroup_root.display(),
            error = %e,
            "Failed to scan cgroup root"
        );
    }

    containers
}

/// Recursively walk the cgroup tree looking for container scopes
fn walk_cgroups(dir: &Path, containers: &mut Vec<Container>) -> io::Result<()> {
    let entries = match fs::read_dir(dir) {
        Ok(entries) => entries,
        Err(e) if e.kind() == io::ErrorKind::PermissionDenied => {
            tracing::debug!(path = %dir.display(), "Permission denied reading cgroup directory");
            return Ok(());
        }
        Err(e) if e.kind() == io::ErrorKind::NotFound => {
            return Ok(());
        }
        Err(e) => return Err(e),
    };

    for entry in entries {
        let entry = entry?;
        let path = entry.path();

        // Only process directories
        if !path.is_dir() {
            continue;
        }

        let name = match entry.file_name().to_str() {
            Some(n) => n.to_string(),
            None => continue,
        };

        // Check if this is a container scope
        if let Some(container) = try_parse_container_scope(&path, &name) {
            containers.push(container);
            // Don't recurse into container scopes - they don't have child cgroups
            continue;
        }

        // Recurse into subdirectories that look like kubepods paths
        // This includes: kubelet.slice, kubelet-kubepods.slice, pod slices, etc.
        if name.ends_with(".slice") || name.ends_with(".scope") || name == "kubelet" {
            walk_cgroups(&path, containers)?;
        }
    }

    Ok(())
}

/// Try to parse a directory as a container scope
///
/// Matches patterns like:
/// - `cri-containerd-<id>.scope`
/// - `crio-<id>.scope`
/// - `docker-<id>.scope`
fn try_parse_container_scope(path: &Path, name: &str) -> Option<Container> {
    // Must be a .scope
    if !name.ends_with(".scope") {
        return None;
    }

    // Try to extract container ID from different CRI patterns
    let (prefix, id) = if let Some(id) = name.strip_prefix("cri-containerd-") {
        ("cri-containerd", id.strip_suffix(".scope")?)
    } else if let Some(id) = name.strip_prefix("crio-") {
        ("crio", id.strip_suffix(".scope")?)
    } else if let Some(id) = name.strip_prefix("docker-") {
        ("docker", id.strip_suffix(".scope")?)
    } else {
        return None;
    };

    // Validate container ID looks reasonable (64 hex chars for containerd/docker)
    if id.is_empty() {
        return None;
    }

    // Read PIDs from cgroup.procs
    let pids = read_cgroup_procs(path);

    // Skip containers with no processes (stopping/stopped)
    if pids.is_empty() {
        tracing::debug!(
            container_id = %id,
            path = %path.display(),
            "Skipping container with no processes"
        );
        return None;
    }

    // Extract pod UID from parent path
    let pod_uid = extract_pod_uid(path);

    // Determine QoS class from path
    let qos_class = QosClass::from_path(path);

    tracing::trace!(
        container_id = %id,
        runtime = %prefix,
        pid_count = pids.len(),
        qos = ?qos_class,
        "Discovered container"
    );

    Some(Container {
        id: id.to_string(),
        cgroup_path: path.to_path_buf(),
        pids,
        pod_uid,
        qos_class,
        // Kubernetes metadata populated later by kubernetes.rs
        pod_name: None,
        container_name: None,
        namespace: None,
        labels: None,
    })
}

/// Read PIDs from cgroup.procs file
fn read_cgroup_procs(cgroup_path: &Path) -> Vec<i32> {
    let procs_path = cgroup_path.join("cgroup.procs");

    let file = match fs::File::open(&procs_path) {
        Ok(f) => f,
        Err(e) => {
            tracing::debug!(
                path = %procs_path.display(),
                error = %e,
                "Failed to open cgroup.procs"
            );
            return Vec::new();
        }
    };

    io::BufReader::new(file)
        .lines()
        .filter_map(|line| {
            line.ok()
                .and_then(|s| s.trim().parse::<i32>().ok())
        })
        .collect()
}

/// Extract pod UID from the cgroup path
///
/// Looks for patterns like:
/// - `kubelet-kubepods-besteffort-pod29b83755_78d3_4345_9a8f_d3017edb5da3.slice`
/// - `kubelet-kubepods-pod29b83755_78d3_4345_9a8f_d3017edb5da3.slice`
///
/// Returns the UID in standard format (dashes, not underscores).
fn extract_pod_uid(container_path: &Path) -> Option<String> {
    // The pod slice is the parent directory of the container scope
    let parent = container_path.parent()?;
    let parent_name = parent.file_name()?.to_str()?;

    // Look for the pod marker and extract everything after it
    let pod_part = if parent_name.contains("-pod") {
        // Find the position of the last "-pod" and extract the UID after it
        let idx = parent_name.rfind("-pod")?;
        &parent_name[idx + 4..] // Skip "-pod"
    } else {
        return None;
    };

    // Remove .slice suffix
    let uid_with_underscores = pod_part.strip_suffix(".slice")?;

    // Convert underscores back to dashes (Kubernetes uses dashes, cgroup uses underscores)
    let uid = uid_with_underscores.replace('_', "-");

    // Validate it looks like a UUID (36 chars with dashes: 8-4-4-4-12)
    if uid.len() == 36 && uid.chars().filter(|c| *c == '-').count() == 4 {
        Some(uid)
    } else {
        tracing::debug!(
            raw = %uid_with_underscores,
            converted = %uid,
            "Pod UID doesn't look like a UUID"
        );
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    #[allow(dead_code)]
    fn make_container(id: &str, pod_uid: Option<&str>, qos: QosClass) -> Container {
        Container {
            id: id.to_string(),
            cgroup_path: std::path::PathBuf::from("/test"),
            pids: vec![1],
            pod_uid: pod_uid.map(String::from),
            qos_class: qos,
            pod_name: None,
            container_name: None,
            namespace: None,
            labels: None,
        }
    }

    fn create_test_cgroup(
        root: &Path,
        path: &str,
        pids: &[i32],
    ) -> io::Result<PathBuf> {
        let full_path = root.join(path);
        fs::create_dir_all(&full_path)?;

        // Write cgroup.procs
        let procs_content: String = pids.iter().map(|p| format!("{}\n", p)).collect();
        fs::write(full_path.join("cgroup.procs"), procs_content)?;

        Ok(full_path)
    }

    #[test]
    fn test_discover_containerd_containers() {
        let tmp = TempDir::new().unwrap();
        let root = tmp.path();

        // Create a realistic cgroup hierarchy for containerd on KIND
        create_test_cgroup(
            root,
            "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/kubelet-kubepods-besteffort-pod29b83755_78d3_4345_9a8f_d3017edb5da3.slice/cri-containerd-19ec54f00502d7236ff37726115196c61a23301c69badddf7838516a03f69e08.scope",
            &[542],
        ).unwrap();

        create_test_cgroup(
            root,
            "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-burstable.slice/kubelet-kubepods-burstable-podabc123_def4_5678_9012_345678901234.slice/cri-containerd-abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890.scope",
            &[1000, 1001, 1002],
        ).unwrap();

        // Guaranteed QoS (no qos subdirectory)
        create_test_cgroup(
            root,
            "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-pod11111111_2222_3333_4444_555555555555.slice/cri-containerd-guaranteed1234567890abcdef1234567890abcdef1234567890abcdef12345678.scope",
            &[2000],
        ).unwrap();

        let containers = scan_cgroups_at(root);

        assert_eq!(containers.len(), 3);

        // Find the besteffort container
        let besteffort = containers
            .iter()
            .find(|c| c.id.starts_with("19ec54f"))
            .expect("Should find besteffort container");
        assert_eq!(besteffort.pids, vec![542]);
        assert_eq!(
            besteffort.pod_uid,
            Some("29b83755-78d3-4345-9a8f-d3017edb5da3".to_string())
        );
        assert_eq!(besteffort.qos_class, QosClass::BestEffort);

        // Find the burstable container
        let burstable = containers
            .iter()
            .find(|c| c.id.starts_with("abcdef"))
            .expect("Should find burstable container");
        assert_eq!(burstable.pids.len(), 3);
        assert_eq!(burstable.qos_class, QosClass::Burstable);

        // Find the guaranteed container
        let guaranteed = containers
            .iter()
            .find(|c| c.id.starts_with("guaranteed"))
            .expect("Should find guaranteed container");
        assert_eq!(guaranteed.qos_class, QosClass::Guaranteed);
    }

    #[test]
    fn test_skip_empty_containers() {
        let tmp = TempDir::new().unwrap();
        let root = tmp.path();

        // Container with no processes (stopping)
        create_test_cgroup(
            root,
            "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/kubelet-kubepods-besteffort-pod12345678_1234_1234_1234_123456789012.slice/cri-containerd-empty1234567890abcdef1234567890abcdef1234567890abcdef1234567890.scope",
            &[], // No PIDs
        ).unwrap();

        // Container with processes
        create_test_cgroup(
            root,
            "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/kubelet-kubepods-besteffort-pod12345678_1234_1234_1234_123456789012.slice/cri-containerd-running1234567890abcdef1234567890abcdef1234567890abcdef123456789.scope",
            &[100],
        ).unwrap();

        let containers = scan_cgroups_at(root);

        // Should only find the running container
        assert_eq!(containers.len(), 1);
        assert!(containers[0].id.starts_with("running"));
    }

    #[test]
    fn test_docker_and_crio_patterns() {
        let tmp = TempDir::new().unwrap();
        let root = tmp.path();

        // Docker container
        create_test_cgroup(
            root,
            "system.slice/docker-abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab.scope",
            &[300],
        ).unwrap();

        // CRI-O container
        create_test_cgroup(
            root,
            "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-burstable.slice/kubelet-kubepods-burstable-pod99999999_8888_7777_6666_555544443333.slice/crio-crio1234567890abcdef1234567890abcdef1234567890abcdef123456789012.scope",
            &[400],
        ).unwrap();

        let containers = scan_cgroups_at(root);

        assert_eq!(containers.len(), 2);

        let docker = containers.iter().find(|c| c.id.starts_with("abcd")).unwrap();
        assert_eq!(docker.pids, vec![300]);

        let crio = containers.iter().find(|c| c.id.starts_with("crio")).unwrap();
        assert_eq!(crio.pids, vec![400]);
    }

    #[test]
    fn test_pod_uid_extraction() {
        // Test various path patterns
        let path = Path::new("/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/kubelet-kubepods-besteffort-pod29b83755_78d3_4345_9a8f_d3017edb5da3.slice/cri-containerd-abc.scope");
        let uid = extract_pod_uid(path);
        assert_eq!(uid, Some("29b83755-78d3-4345-9a8f-d3017edb5da3".to_string()));

        // Guaranteed pod (no qos in path)
        let path = Path::new("/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-pod11111111_2222_3333_4444_555555555555.slice/cri-containerd-abc.scope");
        let uid = extract_pod_uid(path);
        assert_eq!(uid, Some("11111111-2222-3333-4444-555555555555".to_string()));
    }
}
