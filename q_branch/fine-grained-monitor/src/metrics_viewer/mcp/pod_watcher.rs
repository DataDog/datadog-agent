//! Kubernetes pod discovery for fine-grained-monitor DaemonSet.
//!
//! REQ-MCP-007: Discover cluster nodes running fine-grained-monitor pods.
//! REQ-MCP-008: Route requests by node.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};

use k8s_openapi::api::core::v1::Pod;
use kube::api::{Api, ListParams};
use kube::runtime::watcher::{self, Event};
use kube::Client;
use tokio::sync::RwLock;
use tracing::{debug, info, warn};

/// Staleness threshold for individual nodes (60 seconds).
const NODE_STALENESS_THRESHOLD: Duration = Duration::from_secs(60);

/// Staleness threshold for the watcher itself (120 seconds).
const WATCHER_STALENESS_THRESHOLD: Duration = Duration::from_secs(120);

/// Resync interval for the watcher (60 seconds).
const RESYNC_INTERVAL: Duration = Duration::from_secs(60);

/// Information about a DaemonSet pod on a specific node.
#[derive(Debug, Clone)]
pub struct PodInfo {
    /// Kubernetes node name where this pod runs.
    pub node_name: String,
    /// Pod name (e.g., "fine-grained-monitor-abc123").
    pub pod_name: String,
    /// Pod IP address for direct HTTP calls.
    pub pod_ip: String,
    /// Whether the pod is ready (PodReady condition).
    pub ready: bool,
    /// Pod creation timestamp for tie-breaking during rollouts.
    pub creation_timestamp: Option<chrono::DateTime<chrono::Utc>>,
    /// When this pod was last observed in a list/watch event.
    pub last_observed: Instant,
}

/// Node information exposed to agents (sanitized, no pod details).
#[derive(Debug, Clone, serde::Serialize)]
pub struct NodeInfo {
    /// Node name.
    pub name: String,
    /// Whether the pod on this node is ready.
    pub ready: bool,
    /// Whether this node's data is stale.
    pub stale: bool,
    /// Timestamp when pod was last observed (milliseconds since epoch).
    pub last_observed_ms: i64,
}

/// Internal cache state.
#[derive(Debug, Default)]
struct CacheState {
    /// Map from node name to pod info.
    nodes: HashMap<String, PodInfo>,
    /// Last successful Kubernetes sync time.
    last_sync: Option<Instant>,
    /// Epoch time of last sync for external reporting.
    last_sync_epoch_ms: i64,
}

/// Watches fine-grained-monitor DaemonSet pods and maintains node→pod cache.
pub struct PodWatcher {
    /// Kubernetes client.
    client: Client,
    /// Namespace to watch.
    namespace: String,
    /// Label selector for DaemonSet pods.
    label_selector: String,
    /// Viewer HTTP port on pods.
    viewer_port: u16,
    /// Cached pod state.
    cache: Arc<RwLock<CacheState>>,
}

impl PodWatcher {
    /// Create a new PodWatcher.
    pub async fn new(
        namespace: String,
        label_selector: String,
        viewer_port: u16,
    ) -> anyhow::Result<Self> {
        let client = Client::try_default().await?;
        Ok(Self {
            client,
            namespace,
            label_selector,
            viewer_port,
            cache: Arc::new(RwLock::new(CacheState::default())),
        })
    }

    /// Create a PodWatcher with an existing Kubernetes client.
    pub fn with_client(
        client: Client,
        namespace: String,
        label_selector: String,
        viewer_port: u16,
    ) -> Self {
        Self {
            client,
            namespace,
            label_selector,
            viewer_port,
            cache: Arc::new(RwLock::new(CacheState::default())),
        }
    }

    /// Start watching pods. Returns a handle that should be spawned as a task.
    pub async fn start(&self) -> anyhow::Result<()> {
        let pods: Api<Pod> = Api::namespaced(self.client.clone(), &self.namespace);
        let lp = ListParams::default().labels(&self.label_selector);

        // Initial list to populate cache
        self.do_list(&pods, &lp).await?;

        info!(
            namespace = %self.namespace,
            selector = %self.label_selector,
            "PodWatcher started"
        );

        Ok(())
    }

    /// Run the watch loop. This should be spawned as a separate task.
    pub async fn run_watch_loop(self: Arc<Self>) {
        let pods: Api<Pod> = Api::namespaced(self.client.clone(), &self.namespace);
        let lp = ListParams::default().labels(&self.label_selector);

        loop {
            // Use watcher with periodic resync
            let watcher_config = watcher::Config::default().any_semantic();
            let stream = watcher::watcher(pods.clone(), watcher_config);

            use futures::StreamExt;
            // Pin the stream so it can be used in tokio::select!
            tokio::pin!(stream);
            let mut resync_timer = tokio::time::interval(RESYNC_INTERVAL);

            loop {
                tokio::select! {
                    event = stream.next() => {
                        match event {
                            Some(Ok(event)) => {
                                self.handle_event(event).await;
                            }
                            Some(Err(e)) => {
                                warn!(error = %e, "Watch error, will reconnect");
                                break;
                            }
                            None => {
                                info!("Watch stream ended, reconnecting");
                                break;
                            }
                        }
                    }
                    _ = resync_timer.tick() => {
                        // Periodic resync to catch any missed events
                        if let Err(e) = self.do_list(&pods, &lp).await {
                            warn!(error = %e, "Periodic resync failed");
                        }
                    }
                }
            }

            // Backoff before reconnecting
            tokio::time::sleep(Duration::from_secs(5)).await;
        }
    }

    /// Perform a full list and update cache.
    async fn do_list(&self, pods: &Api<Pod>, lp: &ListParams) -> anyhow::Result<()> {
        let pod_list = pods.list(lp).await?;
        let now = Instant::now();
        let epoch_ms = chrono::Utc::now().timestamp_millis();

        let mut cache = self.cache.write().await;

        // Clear and rebuild from list
        cache.nodes.clear();

        for pod in pod_list.items {
            if let Some(info) = self.extract_pod_info(&pod, now) {
                // Handle multiple pods per node (during rollouts)
                let should_insert = cache
                    .nodes
                    .get(&info.node_name)
                    .map(|existing| self.should_replace(existing, &info))
                    .unwrap_or(true);

                if should_insert {
                    debug!(
                        node = %info.node_name,
                        pod = %info.pod_name,
                        ready = info.ready,
                        "Updated node mapping"
                    );
                    cache.nodes.insert(info.node_name.clone(), info);
                }
            }
        }

        cache.last_sync = Some(now);
        cache.last_sync_epoch_ms = epoch_ms;

        info!(node_count = cache.nodes.len(), "Cache synced");
        Ok(())
    }

    /// Handle a watch event.
    async fn handle_event(&self, event: Event<Pod>) {
        let now = Instant::now();
        let epoch_ms = chrono::Utc::now().timestamp_millis();

        match event {
            Event::Apply(pod) | Event::InitApply(pod) => {
                if let Some(info) = self.extract_pod_info(&pod, now) {
                    let mut cache = self.cache.write().await;

                    let should_insert = cache
                        .nodes
                        .get(&info.node_name)
                        .map(|existing| self.should_replace(existing, &info))
                        .unwrap_or(true);

                    if should_insert {
                        debug!(
                            node = %info.node_name,
                            pod = %info.pod_name,
                            ready = info.ready,
                            "Pod applied"
                        );
                        cache.nodes.insert(info.node_name.clone(), info);
                    }

                    cache.last_sync = Some(now);
                    cache.last_sync_epoch_ms = epoch_ms;
                }
            }
            Event::Delete(pod) => {
                let pod_name = pod.metadata.name.as_deref().unwrap_or("unknown");
                let mut cache = self.cache.write().await;

                // Find and remove the node entry if it matches this pod
                let node_to_remove = cache
                    .nodes
                    .iter()
                    .find(|(_, info)| info.pod_name == pod_name)
                    .map(|(node, _)| node.clone());

                if let Some(node) = node_to_remove {
                    debug!(node = %node, pod = %pod_name, "Pod deleted");
                    cache.nodes.remove(&node);
                }

                cache.last_sync = Some(now);
                cache.last_sync_epoch_ms = epoch_ms;
            }
            Event::Init => {
                debug!("Watch init");
            }
            Event::InitDone => {
                debug!("Watch init done");
                let mut cache = self.cache.write().await;
                cache.last_sync = Some(now);
                cache.last_sync_epoch_ms = epoch_ms;
            }
        }
    }

    /// Extract PodInfo from a Pod resource.
    fn extract_pod_info(&self, pod: &Pod, now: Instant) -> Option<PodInfo> {
        let metadata = &pod.metadata;
        let spec = pod.spec.as_ref()?;
        let status = pod.status.as_ref()?;

        let pod_name = metadata.name.as_ref()?.clone();
        let node_name = spec.node_name.as_ref()?.clone();
        let pod_ip = status.pod_ip.as_ref()?.clone();

        // Check PodReady condition
        let ready = status
            .conditions
            .as_ref()
            .and_then(|conditions| {
                conditions
                    .iter()
                    .find(|c| c.type_ == "Ready")
                    .map(|c| c.status == "True")
            })
            .unwrap_or(false);

        let creation_timestamp = metadata.creation_timestamp.as_ref().map(|ts| ts.0);

        Some(PodInfo {
            node_name,
            pod_name,
            pod_ip,
            ready,
            creation_timestamp,
            last_observed: now,
        })
    }

    /// Determine if new pod should replace existing pod for the same node.
    /// Policy: prefer ready pods, then newest creation timestamp.
    fn should_replace(&self, existing: &PodInfo, new: &PodInfo) -> bool {
        // Prefer ready over not ready
        if new.ready && !existing.ready {
            return true;
        }
        if !new.ready && existing.ready {
            return false;
        }

        // Both same readiness - prefer newer
        match (&new.creation_timestamp, &existing.creation_timestamp) {
            (Some(new_ts), Some(existing_ts)) => new_ts > existing_ts,
            (Some(_), None) => true,
            _ => false,
        }
    }

    /// Check if the watcher itself is stale.
    pub async fn is_watcher_stale(&self) -> bool {
        let cache = self.cache.read().await;
        cache
            .last_sync
            .map(|t| t.elapsed() > WATCHER_STALENESS_THRESHOLD)
            .unwrap_or(true)
    }

    /// Get the last sync timestamp in milliseconds.
    pub async fn last_sync_ms(&self) -> i64 {
        self.cache.read().await.last_sync_epoch_ms
    }

    /// List all nodes with their status.
    pub async fn list_nodes(&self) -> Vec<NodeInfo> {
        let cache = self.cache.read().await;

        cache
            .nodes
            .values()
            .map(|pod| {
                let stale = pod.last_observed.elapsed() > NODE_STALENESS_THRESHOLD;
                NodeInfo {
                    name: pod.node_name.clone(),
                    ready: pod.ready,
                    stale,
                    last_observed_ms: chrono::Utc::now().timestamp_millis()
                        - pod.last_observed.elapsed().as_millis() as i64,
                }
            })
            .collect()
    }

    /// Get pod info for a specific node.
    pub async fn get_pod_for_node(&self, node: &str) -> Option<PodInfo> {
        self.cache.read().await.nodes.get(node).cloned()
    }

    /// Get the viewer URL for a specific node.
    pub async fn get_viewer_url(&self, node: &str) -> Option<String> {
        self.get_pod_for_node(node)
            .await
            .map(|pod| format!("http://{}:{}", pod.pod_ip, self.viewer_port))
    }

    /// Get all node names.
    pub async fn node_names(&self) -> Vec<String> {
        self.cache.read().await.nodes.keys().cloned().collect()
    }

    /// Get count of nodes.
    pub async fn node_count(&self) -> usize {
        self.cache.read().await.nodes.len()
    }
}
