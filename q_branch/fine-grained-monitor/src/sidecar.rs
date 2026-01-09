//! Container sidecar files for fast viewer startup.
//!
//! Each parquet file (e.g., `metrics-20251230T120000Z.parquet`) has a companion
//! sidecar file (`metrics-20251230T120000Z.containers`) containing the container
//! metadata active during that file's time window.
//!
//! This enables the viewer to discover containers by scanning tiny sidecar files
//! (~2-5KB) instead of decompressing parquet row groups (~700-800ms per file).
//!
//! Format: bincode-serialized `ContainerSidecar` struct for ~10-100x faster
//! serialization than JSON.

use std::path::Path;

use serde::{Deserialize, Serialize};

/// Sidecar file extension
pub const SIDECAR_EXTENSION: &str = "containers";

/// Container metadata stored in sidecar files.
///
/// Minimal subset of fields needed for viewer display/filtering.
/// Optimized for fast serialization - no timestamps or labels HashMap.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Hash)]
pub struct SidecarContainer {
    /// Short container ID (first 12 chars)
    pub container_id: String,
    /// Pod name (e.g., "coredns-5dd5756b68-abc12")
    pub pod_name: Option<String>,
    /// Container name within pod (e.g., "monitor")
    pub container_name: Option<String>,
    /// Namespace (e.g., "kube-system")
    pub namespace: Option<String>,
    /// Pod UID
    pub pod_uid: Option<String>,
    /// QoS class as string
    pub qos_class: String,
}

/// Sidecar file contents - list of containers active during the parquet file's time window.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContainerSidecar {
    /// Version for forward compatibility
    pub version: u8,
    /// Containers active during this file's time window
    pub containers: Vec<SidecarContainer>,
}

impl ContainerSidecar {
    /// Current sidecar format version
    pub const VERSION: u8 = 1;

    /// Create a new sidecar with the given containers
    pub fn new(containers: Vec<SidecarContainer>) -> Self {
        Self {
            version: Self::VERSION,
            containers,
        }
    }

    /// Write sidecar to file using bincode
    pub fn write(&self, path: &Path) -> Result<(), SidecarError> {
        let tmp_path = path.with_extension("containers.tmp");
        let bytes = bincode::serialize(self)?;
        std::fs::write(&tmp_path, &bytes)?;
        std::fs::rename(&tmp_path, path)?;
        Ok(())
    }

    /// Read sidecar from file using bincode
    pub fn read(path: &Path) -> Result<Self, SidecarError> {
        let bytes = std::fs::read(path)?;
        let sidecar: Self = bincode::deserialize(&bytes)?;

        // Version check for forward compatibility
        if sidecar.version > Self::VERSION {
            return Err(SidecarError::UnsupportedVersion {
                found: sidecar.version,
                max_supported: Self::VERSION,
            });
        }

        Ok(sidecar)
    }
}

/// Get the sidecar path for a parquet file
pub fn sidecar_path_for_parquet(parquet_path: &Path) -> std::path::PathBuf {
    parquet_path.with_extension(SIDECAR_EXTENSION)
}

/// Sidecar file errors
#[derive(Debug, thiserror::Error)]
pub enum SidecarError {
    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("Bincode error: {0}")]
    Bincode(#[from] bincode::Error),

    #[error("Unsupported sidecar version: found {found}, max supported {max_supported}")]
    UnsupportedVersion { found: u8, max_supported: u8 },
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn sample_container(id: &str) -> SidecarContainer {
        SidecarContainer {
            container_id: id.to_string(),
            pod_name: Some(format!("pod-{}", id)),
            container_name: Some("main".to_string()),
            namespace: Some("default".to_string()),
            pod_uid: Some(format!("uid-{}", id)),
            qos_class: "Burstable".to_string(),
        }
    }

    #[test]
    fn test_roundtrip() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("test.containers");

        let original = ContainerSidecar::new(vec![
            sample_container("abc123def456"),
            SidecarContainer {
                container_id: "xyz789abc123".to_string(),
                pod_name: None,
                container_name: None,
                namespace: None,
                pod_uid: None,
                qos_class: "BestEffort".to_string(),
            },
        ]);

        original.write(&path).unwrap();
        let loaded = ContainerSidecar::read(&path).unwrap();

        assert_eq!(loaded.version, ContainerSidecar::VERSION);
        assert_eq!(loaded.containers.len(), 2);
        assert_eq!(loaded.containers[0].container_id, "abc123def456");
        assert_eq!(loaded.containers[1].container_id, "xyz789abc123");
    }

    #[test]
    fn test_roundtrip_empty() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("empty.containers");

        let original = ContainerSidecar::new(vec![]);
        original.write(&path).unwrap();
        let loaded = ContainerSidecar::read(&path).unwrap();

        assert_eq!(loaded.containers.len(), 0);
    }

    #[test]
    fn test_sidecar_path() {
        let parquet = Path::new("/data/metrics-20251230T120000Z.parquet");
        let sidecar = sidecar_path_for_parquet(parquet);
        assert_eq!(
            sidecar,
            Path::new("/data/metrics-20251230T120000Z.containers")
        );
    }

    #[test]
    fn test_unsupported_version() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("future.containers");

        // Manually create a sidecar with a future version
        let future_sidecar = ContainerSidecar {
            version: ContainerSidecar::VERSION + 1,
            containers: vec![sample_container("test123")],
        };
        let bytes = bincode::serialize(&future_sidecar).unwrap();
        std::fs::write(&path, &bytes).unwrap();

        // Reading should fail with version error
        let result = ContainerSidecar::read(&path);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(err, SidecarError::UnsupportedVersion { .. }));
    }

    #[test]
    fn test_corrupt_file() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("corrupt.containers");

        // Write garbage data
        std::fs::write(&path, b"not valid bincode data").unwrap();

        let result = ContainerSidecar::read(&path);
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), SidecarError::Bincode(_)));
    }

    #[test]
    fn test_missing_file() {
        let path = Path::new("/nonexistent/path/test.containers");
        let result = ContainerSidecar::read(path);
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), SidecarError::Io(_)));
    }

    #[test]
    fn test_atomic_write() {
        // Verify atomic write doesn't leave partial files on success
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("atomic.containers");
        let tmp_path = path.with_extension("containers.tmp");

        let sidecar = ContainerSidecar::new(vec![sample_container("test")]);
        sidecar.write(&path).unwrap();

        // Final file should exist, temp file should not
        assert!(path.exists());
        assert!(!tmp_path.exists());
    }
}
