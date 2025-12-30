//! Kubernetes API client for pod metadata enrichment (REQ-PME-002)
//!
//! Queries the Kubernetes API server to obtain pod metadata for containers
//! discovered via cgroup scanning. Uses in-cluster configuration and queries
//! pods filtered by node name to get only local containers.

use std::collections::HashMap;
use std::sync::Arc;

use k8s_openapi::api::core::v1::Pod;
use kube::{
    api::{Api, ListParams},
    Client,
};
use tokio::sync::RwLock;

/// Metadata for a container obtained from Kubernetes API
#[derive(Debug, Clone)]
pub struct PodMetadata {
    /// Pod name (e.g., "coredns-5dd5756b68-abc12")
    pub pod_name: String,
    /// Namespace (e.g., "kube-system")
    pub namespace: String,
    /// Pod labels (optional)
    pub labels: HashMap<String, String>,
}

/// Container ID to pod metadata mapping
pub type ContainerMetadataMap = HashMap<String, PodMetadata>;

/// Kubernetes API client for pod metadata enrichment
pub struct KubernetesClient {
    client: Client,
    node_name: String,
    /// Cached metadata, updated periodically
    cache: Arc<RwLock<ContainerMetadataMap>>,
}

impl KubernetesClient {
    /// Try to create a new Kubernetes client with in-cluster config
    ///
    /// Returns None if Kubernetes API is not available (graceful degradation).
    pub async fn try_new() -> Option<Self> {
        // Get node name from environment (set via downward API in DaemonSet)
        let node_name = match std::env::var("NODE_NAME") {
            Ok(name) => name,
            Err(_) => {
                tracing::info!(
                    "NODE_NAME env var not set, running without Kubernetes metadata enrichment"
                );
                return None;
            }
        };

        // Try to create in-cluster client
        match Client::try_default().await {
            Ok(client) => {
                tracing::info!(
                    node_name = %node_name,
                    "Kubernetes API client initialized for pod metadata enrichment"
                );
                Some(Self {
                    client,
                    node_name,
                    cache: Arc::new(RwLock::new(HashMap::new())),
                })
            }
            Err(e) => {
                tracing::info!(
                    error = %e,
                    "Kubernetes API not available, running without pod metadata enrichment"
                );
                None
            }
        }
    }

    /// Refresh the pod metadata cache by querying the Kubernetes API
    ///
    /// Queries all pods on this node and builds a container ID -> metadata map.
    pub async fn refresh(&self) -> anyhow::Result<()> {
        let pods: Api<Pod> = Api::all(self.client.clone());

        // Query pods on this node only
        let params = ListParams::default()
            .fields(&format!("spec.nodeName={}", self.node_name));

        let pod_list = pods.list(&params).await?;

        let mut new_cache = HashMap::new();

        for pod in pod_list {
            let metadata = pod.metadata;
            let pod_name = match &metadata.name {
                Some(name) => name.clone(),
                None => continue,
            };
            let namespace = metadata.namespace.clone().unwrap_or_default();
            // Convert BTreeMap to HashMap (k8s-openapi uses BTreeMap, we use HashMap)
            let labels: HashMap<String, String> = metadata
                .labels
                .clone()
                .unwrap_or_default()
                .into_iter()
                .collect();

            // Extract container IDs from pod status
            if let Some(status) = &pod.status {
                // Process regular containers
                if let Some(container_statuses) = &status.container_statuses {
                    for cs in container_statuses {
                        if let Some(container_id) = &cs.container_id {
                            let stripped_id = strip_runtime_prefix(container_id);
                            new_cache.insert(
                                stripped_id.to_string(),
                                PodMetadata {
                                    pod_name: pod_name.clone(),
                                    namespace: namespace.clone(),
                                    labels: labels.clone(),
                                },
                            );
                        }
                    }
                }

                // Process init containers
                if let Some(init_statuses) = &status.init_container_statuses {
                    for cs in init_statuses {
                        if let Some(container_id) = &cs.container_id {
                            let stripped_id = strip_runtime_prefix(container_id);
                            new_cache.insert(
                                stripped_id.to_string(),
                                PodMetadata {
                                    pod_name: pod_name.clone(),
                                    namespace: namespace.clone(),
                                    labels: labels.clone(),
                                },
                            );
                        }
                    }
                }

                // Process ephemeral containers
                if let Some(ephemeral_statuses) = &status.ephemeral_container_statuses {
                    for cs in ephemeral_statuses {
                        if let Some(container_id) = &cs.container_id {
                            let stripped_id = strip_runtime_prefix(container_id);
                            new_cache.insert(
                                stripped_id.to_string(),
                                PodMetadata {
                                    pod_name: pod_name.clone(),
                                    namespace: namespace.clone(),
                                    labels: labels.clone(),
                                },
                            );
                        }
                    }
                }
            }
        }

        tracing::debug!(
            containers = new_cache.len(),
            node = %self.node_name,
            "Refreshed Kubernetes pod metadata cache"
        );

        // Update cache
        let mut cache = self.cache.write().await;
        *cache = new_cache;

        Ok(())
    }

    /// Get metadata for a container by its full ID
    pub async fn get_metadata(&self, container_id: &str) -> Option<PodMetadata> {
        let cache = self.cache.read().await;
        cache.get(container_id).cloned()
    }

    /// Get the current cache (for bulk lookups)
    pub async fn get_cache(&self) -> ContainerMetadataMap {
        let cache = self.cache.read().await;
        cache.clone()
    }
}

/// Strip the runtime prefix from a container ID
///
/// Kubernetes API returns container IDs with runtime prefix:
/// - `containerd://abc123def456...`
/// - `docker://abc123def456...`
/// - `cri-o://abc123def456...`
///
/// Strip prefix to match cgroup-discovered IDs.
fn strip_runtime_prefix(id: &str) -> &str {
    id.find("://").map(|i| &id[i + 3..]).unwrap_or(id)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_strip_runtime_prefix() {
        assert_eq!(
            strip_runtime_prefix("containerd://abc123def456"),
            "abc123def456"
        );
        assert_eq!(strip_runtime_prefix("docker://xyz789"), "xyz789");
        assert_eq!(strip_runtime_prefix("cri-o://test123"), "test123");
        assert_eq!(strip_runtime_prefix("plain-id"), "plain-id");
    }
}
