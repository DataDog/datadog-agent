//! MCP server binary for metrics viewer.
//!
//! REQ-MCP-006: Operate via MCP over stdio.
//!
//! # Usage
//!
//! ```bash
//! mcp-metrics-viewer --api-url http://localhost:8050
//! ```
//!
//! # Claude Code Configuration
//!
//! Add to `~/.claude/mcp_settings.json`:
//! ```json
//! {
//!   "mcpServers": {
//!     "metrics-viewer": {
//!       "command": "/path/to/mcp-metrics-viewer",
//!       "args": ["--api-url", "http://localhost:8050"]
//!     }
//!   }
//! }
//! ```

use anyhow::Result;
use clap::Parser;
use fine_grained_monitor::metrics_viewer::mcp::{client::MetricsViewerClient, McpMetricsViewer};
use rmcp::{transport::stdio, ServiceExt};

#[derive(Parser, Debug)]
#[command(name = "mcp-metrics-viewer")]
#[command(about = "MCP server for container metrics analysis")]
#[command(version)]
struct Args {
    /// Base URL of the metrics-viewer HTTP API.
    /// Example: http://localhost:8050
    #[arg(long, env = "METRICS_VIEWER_API_URL")]
    api_url: String,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Parse CLI args
    let args = Args::parse();

    // Create HTTP client
    let client = MetricsViewerClient::new(&args.api_url)?;

    // Create MCP server
    let server = McpMetricsViewer::new(client);

    // Run with stdio transport (REQ-MCP-006)
    let service = server.serve(stdio()).await.map_err(|e| {
        anyhow::anyhow!("Failed to start MCP server: {}", e)
    })?;

    // Wait for completion
    service.waiting().await.map_err(|e| {
        anyhow::anyhow!("MCP server error: {}", e)
    })?;

    Ok(())
}
