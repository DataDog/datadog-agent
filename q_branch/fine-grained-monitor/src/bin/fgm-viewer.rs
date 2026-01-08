//! CLI binary for the metrics viewer.
//!
//! REQ-MV-001: Loads parquet file and serves HTTP on configurable port.
//! REQ-MV-011: Supports directory input with glob for `*.parquet` files.
//!
//! # Usage
//!
//! ```bash
//! fgm-viewer metrics.parquet
//! fgm-viewer metrics.parquet --port 8080
//! fgm-viewer metrics.parquet --no-browser
//! fgm-viewer data/*.parquet  # Multiple files (shell expansion)
//! fgm-viewer /data           # Directory input (scans parquet files directly)
//! ```

use anyhow::Result;
use clap::Parser;
use fine_grained_monitor::metrics_viewer::{server, LazyDataStore};
use glob::glob;
use std::path::PathBuf;
use std::time::Duration;
use tracing_subscriber;

#[derive(Parser, Debug)]
#[command(name = "fgm-viewer")]
#[command(about = "Interactive metrics viewer with web frontend")]
#[command(version)]
struct Args {
    /// Input parquet file(s) or directory
    /// If a directory is provided, scans parquet files directly for metadata
    #[arg(required = true)]
    input: Vec<PathBuf>,

    /// Port for web server
    #[arg(short, long, default_value = "8050")]
    port: u16,

    /// Don't open browser automatically
    #[arg(long)]
    no_browser: bool,

    /// Timeout in seconds for waiting for parquet files to appear (default: 180 = 3 minutes)
    #[arg(long, default_value = "180")]
    timeout_secs: u64,
}

/// Wait for parquet files to appear in the data directory.
/// Returns true if files are found, false on timeout.
fn wait_for_parquet_files(data_dir: &PathBuf, timeout: Duration) -> bool {
    let start = std::time::Instant::now();
    let poll_interval = Duration::from_secs(5);
    let pattern = format!("{}/**/*.parquet", data_dir.display());

    eprintln!("Looking for parquet files in {:?}", data_dir);

    loop {
        // Check for parquet files
        let file_count = glob(&pattern)
            .map(|paths| paths.filter_map(Result::ok).take(1).count())
            .unwrap_or(0);

        if file_count > 0 {
            return true;
        }

        // Check timeout
        if start.elapsed() >= timeout {
            eprintln!("Timeout waiting for parquet files");
            return false;
        }

        eprintln!(
            "No parquet files yet, waiting... ({:.0}s / {:.0}s)",
            start.elapsed().as_secs_f64(),
            timeout.as_secs_f64()
        );
        std::thread::sleep(poll_interval);
    }
}

/// REQ-MV-011: Expand input paths, handling directories by globbing for parquet files.
/// Used for legacy mode (explicit file list) when not using index.
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
    // Initialize tracing (respects RUST_LOG env var)
    tracing_subscriber::fmt::init();

    let args = Args::parse();
    let timeout = Duration::from_secs(args.timeout_secs);

    let config = server::ServerConfig {
        port: args.port,
        open_browser: !args.no_browser,
    };

    // Check if input is a single directory (directory scan mode)
    let is_directory_mode = args.input.len() == 1 && args.input[0].is_dir();

    let store = if is_directory_mode {
        // Directory mode: scan parquet files directly for metadata
        let data_dir = &args.input[0];
        eprintln!("Directory mode: scanning parquet files from {:?}", data_dir);

        // Wait for parquet files to appear (collector might still be starting)
        if !wait_for_parquet_files(data_dir, timeout) {
            anyhow::bail!("No parquet files found in {:?} after {}s", data_dir, timeout.as_secs());
        }

        // Create store by scanning parquet files directly (no index.json needed)
        LazyDataStore::from_directory(data_dir.clone())?
    } else {
        // Legacy mode: explicit file list
        let files = expand_inputs(&args.input)?;

        if files.is_empty() {
            eprintln!("No parquet files found in {:?}", args.input);
            eprintln!("Waiting for data to be collected...");
        } else {
            eprintln!("Found {} parquet file(s)", files.len());
            for f in files.iter().take(10) {
                eprintln!("  {}", f.display());
            }
            if files.len() > 10 {
                eprintln!("  ... and {} more", files.len() - 10);
            }
        }

        LazyDataStore::new(&files)?
    };

    server::run_server(store, config).await?;

    Ok(())
}
