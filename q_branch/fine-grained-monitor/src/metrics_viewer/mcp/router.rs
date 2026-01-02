//! Node→pod routing with staleness checking and retry logic.
//!
//! REQ-MCP-008: Route requests by node.

use std::sync::Arc;
use std::time::Duration;

use anyhow::Result;
use reqwest::Client;
use tracing::{debug, warn};

use super::pod_watcher::PodWatcher;

/// Operation-specific timeouts.
const TIMEOUT_LIST_METRICS: Duration = Duration::from_secs(5);
const TIMEOUT_LIST_CONTAINERS: Duration = Duration::from_secs(5);
const TIMEOUT_ANALYZE: Duration = Duration::from_secs(30);

/// Retry jitter range (50-200ms).
const RETRY_JITTER_MIN_MS: u64 = 50;
const RETRY_JITTER_MAX_MS: u64 = 200;

/// Routes requests to the correct viewer pod based on node.
pub struct NodeRouter {
    pod_watcher: Arc<PodWatcher>,
    http_client: Client,
}

/// Error types for routing operations.
#[derive(Debug, thiserror::Error)]
pub enum RouterError {
    #[error("watcher is stale (last sync was too long ago)")]
    WatcherStale,

    #[error("node '{0}' not found")]
    NodeNotFound(String),

    #[error("node '{0}' is not ready")]
    NodeNotReady(String),

    #[error("node '{0}' is stale (last seen {1}s ago)")]
    NodeStale(String, u64),

    #[error("request to node '{0}' timed out")]
    Timeout(String),

    #[error("request to node '{0}' failed: {1}")]
    RequestFailed(String, String),

    #[error("viewer error on node '{0}': {1} {2}")]
    ViewerError(String, u16, String),
}

impl NodeRouter {
    /// Create a new router with the given pod watcher.
    pub fn new(pod_watcher: Arc<PodWatcher>) -> Self {
        let http_client = Client::builder()
            .pool_max_idle_per_host(10)
            .build()
            .expect("Failed to create HTTP client");

        Self {
            pod_watcher,
            http_client,
        }
    }

    /// Check if the watcher is healthy before any operation.
    pub async fn check_watcher_health(&self) -> Result<(), RouterError> {
        if self.pod_watcher.is_watcher_stale().await {
            return Err(RouterError::WatcherStale);
        }
        Ok(())
    }

    /// Validate that a node exists and is ready for requests.
    pub async fn validate_node(&self, node: &str) -> Result<String, RouterError> {
        // Check watcher health first
        self.check_watcher_health().await?;

        // Get pod info for node
        let pod = self
            .pod_watcher
            .get_pod_for_node(node)
            .await
            .ok_or_else(|| RouterError::NodeNotFound(node.to_string()))?;

        // Check readiness
        if !pod.ready {
            return Err(RouterError::NodeNotReady(node.to_string()));
        }

        // Check staleness
        let elapsed_secs = pod.last_observed.elapsed().as_secs();
        if elapsed_secs > 60 {
            return Err(RouterError::NodeStale(node.to_string(), elapsed_secs));
        }

        // Return the viewer URL
        Ok(format!("http://{}:8050", pod.pod_ip))
    }

    /// Execute a GET request to a node's viewer.
    pub async fn get(
        &self,
        node: &str,
        path: &str,
        timeout: Duration,
    ) -> Result<String, RouterError> {
        // First attempt
        match self.do_get(node, path, timeout).await {
            Ok(body) => return Ok(body),
            Err(e) => {
                // Don't retry on 4xx errors
                if matches!(e, RouterError::ViewerError(_, status, _) if status >= 400 && status < 500)
                {
                    return Err(e);
                }
                warn!(node = %node, error = %e, "First request failed, retrying");
            }
        }

        // Add jitter before retry
        let jitter = rand_jitter();
        tokio::time::sleep(Duration::from_millis(jitter)).await;

        // Retry with re-resolution
        self.do_get(node, path, timeout).await
    }

    /// Internal GET implementation.
    async fn do_get(&self, node: &str, path: &str, timeout: Duration) -> Result<String, RouterError> {
        let base_url = self.validate_node(node).await?;
        let url = format!("{}{}", base_url, path);

        debug!(node = %node, url = %url, "Sending request");

        let response = self
            .http_client
            .get(&url)
            .timeout(timeout)
            .send()
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    RouterError::Timeout(node.to_string())
                } else {
                    RouterError::RequestFailed(node.to_string(), e.to_string())
                }
            })?;

        let status = response.status();
        let body = response.text().await.map_err(|e| {
            RouterError::RequestFailed(node.to_string(), format!("Failed to read body: {}", e))
        })?;

        if !status.is_success() {
            return Err(RouterError::ViewerError(
                node.to_string(),
                status.as_u16(),
                body,
            ));
        }

        Ok(body)
    }

    /// Get timeout for list_metrics operation.
    pub fn timeout_list_metrics() -> Duration {
        TIMEOUT_LIST_METRICS
    }

    /// Get timeout for list_containers operation.
    pub fn timeout_list_containers() -> Duration {
        TIMEOUT_LIST_CONTAINERS
    }

    /// Get timeout for analyze operation.
    pub fn timeout_analyze() -> Duration {
        TIMEOUT_ANALYZE
    }
}

/// Generate random jitter in the range [50, 200] ms.
fn rand_jitter() -> u64 {
    use std::time::SystemTime;
    let seed = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap()
        .subsec_nanos() as u64;
    RETRY_JITTER_MIN_MS + (seed % (RETRY_JITTER_MAX_MS - RETRY_JITTER_MIN_MS))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_rand_jitter_in_range() {
        for _ in 0..100 {
            let jitter = rand_jitter();
            assert!(jitter >= RETRY_JITTER_MIN_MS);
            assert!(jitter < RETRY_JITTER_MAX_MS);
        }
    }
}
