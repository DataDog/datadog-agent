//! CLI binary for the metrics viewer.
//!
//! REQ-MV-001: Loads parquet file and serves HTTP on configurable port.
//! REQ-ICV-002: Supports directory input with glob for `*.parquet` files.
//!
//! # Usage
//!
//! ```bash
//! fgm-viewer metrics.parquet
//! fgm-viewer metrics.parquet --port 8080
//! fgm-viewer metrics.parquet --no-browser
//! fgm-viewer data/*.parquet  # Multiple files (shell expansion)
//! fgm-viewer /data           # Directory input (globs for *.parquet)
//! ```

use anyhow::Result;
use clap::Parser;
use fine_grained_monitor::metrics_viewer::{server, LazyDataStore};
use glob::glob;
use std::path::PathBuf;

#[derive(Parser, Debug)]
#[command(name = "fgm-viewer")]
#[command(about = "Interactive metrics viewer with web frontend")]
#[command(version)]
struct Args {
    /// Input parquet file(s) or directory
    /// If a directory is provided, globs for **/*.parquet recursively
    #[arg(required = true)]
    input: Vec<PathBuf>,

    /// Port for web server
    #[arg(short, long, default_value = "8050")]
    port: u16,

    /// Don't open browser automatically
    #[arg(long)]
    no_browser: bool,
}

/// REQ-ICV-002: Expand input paths, handling directories by globbing for parquet files.
fn expand_inputs(inputs: &[PathBuf]) -> Result<Vec<PathBuf>> {
    let mut files = Vec::new();

    for path in inputs {
        if path.is_dir() {
            // Directory: glob for all parquet files recursively
            let pattern = format!("{}/**/*.parquet", path.display());
            eprintln!("Searching for parquet files: {}", pattern);

            for entry in glob(&pattern)? {
                match entry {
                    Ok(file_path) => files.push(file_path),
                    Err(e) => eprintln!("Warning: glob error: {}", e),
                }
            }
        } else if path.exists() {
            // Regular file
            files.push(path.clone());
        } else {
            // Path doesn't exist - might be a glob pattern from shell
            // Try to glob it in case shell didn't expand
            let pattern = path.to_string_lossy();
            let mut found = false;
            for entry in glob(&pattern)? {
                match entry {
                    Ok(file_path) => {
                        files.push(file_path);
                        found = true;
                    }
                    Err(e) => eprintln!("Warning: glob error: {}", e),
                }
            }
            if !found {
                eprintln!("Warning: no files found matching {:?}", path);
            }
        }
    }

    // Sort by modification time (newest first) for consistent ordering
    files.sort_by(|a, b| {
        let a_time = a.metadata().and_then(|m| m.modified()).ok();
        let b_time = b.metadata().and_then(|m| m.modified()).ok();
        b_time.cmp(&a_time)
    });

    Ok(files)
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    // REQ-ICV-002: Expand directory inputs to parquet files
    let files = expand_inputs(&args.input)?;

    // REQ-ICV-003: Handle empty file list gracefully
    if files.is_empty() {
        eprintln!("No parquet files found in {:?}", args.input);
        eprintln!("Waiting for data to be collected...");
        eprintln!("The viewer will serve an empty state until files appear.");
    } else {
        eprintln!("Found {} parquet file(s)", files.len());
        for f in &files {
            eprintln!("  {}", f.display());
        }
    }

    let config = server::ServerConfig {
        port: args.port,
        open_browser: !args.no_browser,
    };

    // Lazy loading - fast startup, on-demand data loading
    // REQ-ICV-003: LazyDataStore handles empty file list by returning empty index
    let store = LazyDataStore::new(&files)?;
    server::run_server(store, config).await?;

    Ok(())
}
