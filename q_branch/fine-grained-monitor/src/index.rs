//! Container metadata tracking for sidecar generation.
//!
//! The collector maintains an in-memory index of container metadata.
//! This is used to generate sidecar files at rotation time, enabling
//! the viewer to discover containers without scanning parquet files.

use std::collections::HashMap;

use crate::discovery::{Container, QosClass};

/// Container metadata for sidecar generation.
///
/// Contains only the fields needed for viewer display/filtering.
#[derive(Debug, Clone)]
pub struct ContainerEntry {
    /// Pod UID (if available)
    pub pod_uid: Option<String>,
    /// QoS class (Guaranteed, Burstable, BestEffort)
    pub qos_class: String,
    /// Pod name from Kubernetes API (e.g., "coredns-5dd5756b68-abc12")
    pub pod_name: Option<String>,
    /// Container name within the pod (e.g., "monitor", "viewer", "consolidator")
    pub container_name: Option<String>,
    /// Namespace from Kubernetes API (e.g., "kube-system")
    pub namespace: Option<String>,
    /// Pod labels from Kubernetes API
    pub labels: Option<HashMap<String, String>>,
}

/// In-memory container metadata index for sidecar generation.
#[derive(Debug, Clone)]
pub struct ContainerIndex {
    /// Container metadata keyed by short ID (first 12 chars)
    pub containers: HashMap<String, ContainerEntry>,
}

impl ContainerIndex {
    /// Create a new empty index
    #[allow(unused_variables)]
    pub fn new(rotation_interval_sec: u64) -> Self {
        Self {
            containers: HashMap::new(),
        }
    }

    /// Update index with current container set
    pub fn update(&mut self, containers: &[Container]) {
        // Add new containers or update existing ones with Kubernetes metadata
        for container in containers {
            let sid = short_id(&container.id);
            if let Some(entry) = self.containers.get_mut(&sid) {
                // Update existing container with new Kubernetes metadata if available
                let needs_pod_name = container.pod_name.is_some() && entry.pod_name.is_none();
                let needs_container_name =
                    container.container_name.is_some() && entry.container_name.is_none();
                let needs_labels = container.labels.is_some() && entry.labels.is_none();
                if needs_pod_name || needs_container_name || needs_labels {
                    entry.pod_name = entry.pod_name.clone().or(container.pod_name.clone());
                    entry.container_name =
                        entry.container_name.clone().or(container.container_name.clone());
                    entry.namespace = entry.namespace.clone().or(container.namespace.clone());
                    entry.labels = entry.labels.clone().or(container.labels.clone());
                    tracing::info!(
                        container_id = %sid,
                        pod_name = ?entry.pod_name,
                        container_name = ?entry.container_name,
                        namespace = ?entry.namespace,
                        labels_count = entry.labels.as_ref().map(|l| l.len()).unwrap_or(0),
                        "Updated container with Kubernetes metadata"
                    );
                }
            } else {
                self.containers.insert(
                    sid.clone(),
                    ContainerEntry {
                        pod_uid: container.pod_uid.clone(),
                        qos_class: qos_to_string(container.qos_class),
                        pod_name: container.pod_name.clone(),
                        container_name: container.container_name.clone(),
                        namespace: container.namespace.clone(),
                        labels: container.labels.clone(),
                    },
                );
                tracing::info!(
                    container_id = %sid,
                    pod_uid = ?container.pod_uid,
                    pod_name = ?container.pod_name,
                    container_name = ?container.container_name,
                    qos_class = %qos_to_string(container.qos_class),
                    "New container discovered"
                );
            }
        }
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

    fn make_container(id: &str, pod_uid: Option<&str>, qos: QosClass) -> Container {
        Container {
            id: id.to_string(),
            cgroup_path: PathBuf::from("/test"),
            pids: vec![1],
            pod_uid: pod_uid.map(String::from),
            qos_class: qos,
            pod_name: None,
            container_name: None,
            namespace: None,
            labels: None,
        }
    }

    #[test]
    fn test_new_index() {
        let index = ContainerIndex::new(90);
        assert!(index.containers.is_empty());
    }

    #[test]
    fn test_update_adds_new_containers() {
        let mut index = ContainerIndex::new(90);

        let containers = vec![
            make_container("abc123def456789", Some("pod-uid-1"), QosClass::Burstable),
            make_container("xyz789abc123456", None, QosClass::Guaranteed),
        ];

        index.update(&containers);
        assert_eq!(index.containers.len(), 2);
        assert!(index.containers.contains_key("abc123def456"));
        assert!(index.containers.contains_key("xyz789abc123"));
    }

    #[test]
    fn test_update_same_containers() {
        let mut index = ContainerIndex::new(90);

        let containers = vec![make_container(
            "abc123def456789",
            None,
            QosClass::Burstable,
        )];

        index.update(&containers);
        index.update(&containers);

        assert_eq!(index.containers.len(), 1);
    }
}
