//! MCP server binary for metrics viewer.
//!
//! REQ-MCP-006: Operate via MCP over HTTP/SSE.
//!
//! Runs as an in-cluster Deployment, discovering fine-grained-monitor DaemonSet
//! pods and routing node-targeted queries to the correct pod.
//!
//! # Usage
//!
//! ```bash
//! mcp-metrics-viewer --port 8080 --daemonset-namespace default --daemonset-label app=fine-grained-monitor
//! ```
//!
//! # Claude Code Configuration
//!
//! After port-forwarding the MCP service:
//! ```bash
//! kubectl port-forward svc/mcp-metrics 8888:8080
//! ```
//!
//! Configure MCP client to connect to `http://localhost:8888/mcp`

use std::sync::Arc;

use anyhow::Result;
use clap::Parser;
use fine_grained_monitor::metrics_viewer::mcp::{pod_watcher::PodWatcher, McpMetricsViewer};
use rmcp::transport::{
    StreamableHttpServerConfig,
    streamable_http_server::{session::local::LocalSessionManager, StreamableHttpService},
};
use tokio_util::sync::CancellationToken;
use tracing::{info, Level};
use tracing_subscriber::FmtSubscriber;

#[derive(Parser, Debug)]
#[command(name = "mcp-metrics-viewer")]
#[command(about = "MCP server for container metrics analysis (in-cluster)")]
#[command(version)]
struct Args {
    /// HTTP/SSE listen port.
    #[arg(long, default_value = "8080", env = "MCP_PORT")]
    port: u16,

    /// Namespace containing the DaemonSet pods.
    #[arg(long, default_value = "default", env = "DAEMONSET_NAMESPACE")]
    daemonset_namespace: String,

    /// Label selector for DaemonSet pods.
    #[arg(long, default_value = "app=fine-grained-monitor", env = "DAEMONSET_LABEL")]
    daemonset_label: String,

    /// HTTP port on viewer pods.
    #[arg(long, default_value = "8050", env = "VIEWER_PORT")]
    viewer_port: u16,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    FmtSubscriber::builder()
        .with_max_level(Level::INFO)
        .with_target(false)
        .json()
        .init();

    // Parse CLI args
    let args = Args::parse();

    info!(
        port = args.port,
        namespace = %args.daemonset_namespace,
        label = %args.daemonset_label,
        viewer_port = args.viewer_port,
        "Starting MCP metrics viewer"
    );

    // Create pod watcher
    let pod_watcher = Arc::new(
        PodWatcher::new(
            args.daemonset_namespace.clone(),
            args.daemonset_label.clone(),
            args.viewer_port,
        )
        .await?,
    );

    // Start the watcher
    pod_watcher.start().await?;

    // Spawn watch loop in background
    let watcher_clone = pod_watcher.clone();
    tokio::spawn(async move {
        watcher_clone.run_watch_loop().await;
    });

    // Setup HTTP server with Streamable HTTP transport (REQ-MCP-006)
    let ct = CancellationToken::new();

    // Create the MCP service factory - each session gets its own McpMetricsViewer
    // sharing the same PodWatcher
    let pod_watcher_for_factory = pod_watcher.clone();
    let service: StreamableHttpService<McpMetricsViewer, LocalSessionManager> =
        StreamableHttpService::new(
            move || Ok(McpMetricsViewer::new(pod_watcher_for_factory.clone())),
            Default::default(),
            StreamableHttpServerConfig {
                stateful_mode: true,
                sse_keep_alive: Some(std::time::Duration::from_secs(15)),
                cancellation_token: ct.child_token(),
            },
        );

    // Create axum router with MCP endpoint and health check
    let router = axum::Router::new()
        .nest_service("/mcp", service)
        .route("/health", axum::routing::get(|| async { "ok" }))
        .route("/ready", axum::routing::get(|| async { "ok" }));

    // Bind and serve
    let bind_addr = format!("0.0.0.0:{}", args.port);
    info!(bind = %bind_addr, "Starting HTTP/SSE server");

    let tcp_listener = tokio::net::TcpListener::bind(&bind_addr).await?;

    // Handle shutdown signals
    let ct_shutdown = ct.clone();
    tokio::spawn(async move {
        tokio::signal::ctrl_c().await.ok();
        info!("Received shutdown signal");
        ct_shutdown.cancel();
    });

    // Run the server
    axum::serve(tcp_listener, router)
        .with_graceful_shutdown(async move { ct.cancelled().await })
        .await?;

    info!("Server shutdown complete");
    Ok(())
}
