//! Container index for fast viewer startup (REQ-ICV-003)
//!
//! The collector maintains a lightweight index.json with container metadata,
//! enabling the viewer to start instantly without scanning all parquet files.

use std::collections::HashMap;
use std::path::Path;

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

use crate::discovery::{Container, QosClass};

/// Current schema version for forward compatibility
const SCHEMA_VERSION: u32 = 1;

/// Container metadata stored in the index
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContainerEntry {
    /// Full container ID (64 hex chars)
    pub full_id: String,
    /// Pod UID (if available)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pod_uid: Option<String>,
    /// QoS class (Guaranteed, Burstable, BestEffort)
    pub qos_class: String,
    /// When this container was first observed
    pub first_seen: DateTime<Utc>,
    /// When this container was last observed
    pub last_seen: DateTime<Utc>,
}

/// Data file time range information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DataRange {
    /// Earliest data file timestamp
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earliest: Option<DateTime<Utc>>,
    /// Latest data file timestamp
    pub latest: DateTime<Utc>,
    /// Rotation interval in seconds (for computing file paths)
    pub rotation_interval_sec: u64,
}

/// The container index written by the collector
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContainerIndex {
    /// Schema version for compatibility
    pub schema_version: u32,
    /// When the index was last updated
    pub updated_at: DateTime<Utc>,
    /// Container metadata keyed by short ID (first 12 chars)
    pub containers: HashMap<String, ContainerEntry>,
    /// Data file time range
    pub data_range: DataRange,
}

impl ContainerIndex {
    /// Create a new empty index
    pub fn new(rotation_interval_sec: u64) -> Self {
        Self {
            schema_version: SCHEMA_VERSION,
            updated_at: Utc::now(),
            containers: HashMap::new(),
            data_range: DataRange {
                earliest: None,
                latest: Utc::now(),
                rotation_interval_sec,
            },
        }
    }

    /// Load index from file, or create new if missing/invalid
    pub fn load_or_create(path: &Path, rotation_interval_sec: u64) -> Self {
        match Self::load(path) {
            Ok(index) => {
                tracing::info!(
                    containers = index.containers.len(),
                    updated_at = %index.updated_at,
                    "Loaded existing index"
                );
                index
            }
            Err(e) => {
                tracing::info!(error = %e, "Creating new index");
                Self::new(rotation_interval_sec)
            }
        }
    }

    /// Load index from file
    pub fn load(path: &Path) -> anyhow::Result<Self> {
        let content = std::fs::read_to_string(path)?;
        let index: Self = serde_json::from_str(&content)?;

        // Check schema version
        if index.schema_version > SCHEMA_VERSION {
            anyhow::bail!(
                "Index schema version {} is newer than supported {}",
                index.schema_version,
                SCHEMA_VERSION
            );
        }

        Ok(index)
    }

    /// Write index to file atomically
    pub fn save(&self, path: &Path) -> anyhow::Result<()> {
        let tmp_path = path.with_extension("json.tmp");
        let json = serde_json::to_string_pretty(self)?;

        std::fs::write(&tmp_path, &json)?;
        std::fs::rename(&tmp_path, path)?;

        tracing::debug!(
            path = %path.display(),
            containers = self.containers.len(),
            "Saved index"
        );

        Ok(())
    }

    /// Update index with current container set, returns true if changes were made
    pub fn update(&mut self, containers: &[Container]) -> bool {
        let now = Utc::now();
        let mut changed = false;

        // Build set of current container short IDs
        let current_ids: std::collections::HashSet<String> = containers
            .iter()
            .map(|c| short_id(&c.id))
            .collect();

        // Update existing containers' last_seen if still present
        for (short_id, entry) in self.containers.iter_mut() {
            if current_ids.contains(short_id) {
                entry.last_seen = now;
            }
        }

        // Add new containers
        for container in containers {
            let sid = short_id(&container.id);
            if !self.containers.contains_key(&sid) {
                self.containers.insert(
                    sid.clone(),
                    ContainerEntry {
                        full_id: container.id.clone(),
                        pod_uid: container.pod_uid.clone(),
                        qos_class: qos_to_string(container.qos_class),
                        first_seen: now,
                        last_seen: now,
                    },
                );
                tracing::info!(
                    container_id = %sid,
                    pod_uid = ?container.pod_uid,
                    qos_class = %qos_to_string(container.qos_class),
                    "New container discovered"
                );
                changed = true;
            }
        }

        if changed {
            self.updated_at = now;
        }

        changed
    }

    /// Update the data range (called on rotation)
    pub fn update_data_range(&mut self, latest: DateTime<Utc>) {
        if self.data_range.earliest.is_none() {
            self.data_range.earliest = Some(latest);
        }
        self.data_range.latest = latest;
        self.updated_at = Utc::now();
    }
}

/// Extract short container ID (first 12 characters)
fn short_id(full_id: &str) -> String {
    if full_id.len() > 12 {
        full_id[..12].to_string()
    } else {
        full_id.to_string()
    }
}

/// Convert QosClass enum to string
fn qos_to_string(qos: QosClass) -> String {
    match qos {
        QosClass::Guaranteed => "Guaranteed".to_string(),
        QosClass::Burstable => "Burstable".to_string(),
        QosClass::BestEffort => "BestEffort".to_string(),
        QosClass::Unknown => "Unknown".to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use tempfile::TempDir;

    fn make_container(id: &str, pod_uid: Option<&str>, qos: QosClass) -> Container {
        Container {
            id: id.to_string(),
            cgroup_path: PathBuf::from("/test"),
            pids: vec![1],
            pod_uid: pod_uid.map(String::from),
            qos_class: qos,
        }
    }

    #[test]
    fn test_new_index() {
        let index = ContainerIndex::new(90);
        assert_eq!(index.schema_version, SCHEMA_VERSION);
        assert!(index.containers.is_empty());
        assert_eq!(index.data_range.rotation_interval_sec, 90);
    }

    #[test]
    fn test_update_adds_new_containers() {
        let mut index = ContainerIndex::new(90);

        let containers = vec![
            make_container("abc123def456789", Some("pod-uid-1"), QosClass::Burstable),
            make_container("xyz789abc123456", None, QosClass::Guaranteed),
        ];

        let changed = index.update(&containers);
        assert!(changed);
        assert_eq!(index.containers.len(), 2);
        assert!(index.containers.contains_key("abc123def456"));
        assert!(index.containers.contains_key("xyz789abc123"));
    }

    #[test]
    fn test_update_no_change_same_containers() {
        let mut index = ContainerIndex::new(90);

        let containers = vec![make_container(
            "abc123def456789",
            None,
            QosClass::Burstable,
        )];

        index.update(&containers);
        let changed = index.update(&containers);

        assert!(!changed); // No new containers
        assert_eq!(index.containers.len(), 1);
    }

    #[test]
    fn test_save_and_load() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("index.json");

        let mut index = ContainerIndex::new(90);
        index.update(&[make_container(
            "test123456789012",
            Some("pod-123"),
            QosClass::BestEffort,
        )]);

        index.save(&path).unwrap();

        let loaded = ContainerIndex::load(&path).unwrap();
        assert_eq!(loaded.containers.len(), 1);
        assert!(loaded.containers.contains_key("test12345678"));
    }
}
