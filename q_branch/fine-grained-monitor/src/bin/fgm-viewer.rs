//! CLI binary for the metrics viewer.
//!
//! REQ-MV-001: Loads parquet file and serves HTTP on configurable port.
//!
//! # Usage
//!
//! ```bash
//! fgm-viewer metrics.parquet
//! fgm-viewer metrics.parquet --port 8080
//! fgm-viewer metrics.parquet --no-browser
//! fgm-viewer data/*.parquet  # Multiple files
//! ```

use anyhow::Result;
use clap::Parser;
use fine_grained_monitor::metrics_viewer::{server, LazyDataStore};
use std::path::PathBuf;

#[derive(Parser, Debug)]
#[command(name = "fgm-viewer")]
#[command(about = "Interactive metrics viewer with web frontend")]
#[command(version)]
struct Args {
    /// Input parquet file(s)
    #[arg(required = true)]
    input: Vec<PathBuf>,

    /// Port for web server
    #[arg(short, long, default_value = "8050")]
    port: u16,

    /// Don't open browser automatically
    #[arg(long)]
    no_browser: bool,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    // Validate input files exist
    for path in &args.input {
        if !path.exists() {
            anyhow::bail!("File not found: {:?}", path);
        }
    }

    let config = server::ServerConfig {
        port: args.port,
        open_browser: !args.no_browser,
    };

    // Lazy loading - fast startup, on-demand data loading
    let store = LazyDataStore::new(&args.input)?;
    server::run_server(store, config).await?;

    Ok(())
}
