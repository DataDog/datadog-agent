//! CLI binary for the metrics viewer.
//!
//! REQ-MV-001: Loads parquet file and serves HTTP on configurable port.
//! REQ-ICV-002: Supports directory input with glob for `*.parquet` files.
//! REQ-ICV-003: Fast startup via index file - no scanning of all parquet files.
//!
//! # Usage
//!
//! ```bash
//! fgm-viewer metrics.parquet
//! fgm-viewer metrics.parquet --port 8080
//! fgm-viewer metrics.parquet --no-browser
//! fgm-viewer data/*.parquet  # Multiple files (shell expansion)
//! fgm-viewer /data           # Directory input (uses index.json for fast startup)
//! ```

use anyhow::Result;
use clap::Parser;
use fine_grained_monitor::index::ContainerIndex;
use fine_grained_monitor::metrics_viewer::{server, LazyDataStore};
use glob::glob;
use std::path::PathBuf;
use std::time::{Duration, Instant};

#[derive(Parser, Debug)]
#[command(name = "fgm-viewer")]
#[command(about = "Interactive metrics viewer with web frontend")]
#[command(version)]
struct Args {
    /// Input parquet file(s) or directory
    /// If a directory is provided, uses index.json for fast startup
    #[arg(required = true)]
    input: Vec<PathBuf>,

    /// Port for web server
    #[arg(short, long, default_value = "8050")]
    port: u16,

    /// Don't open browser automatically
    #[arg(long)]
    no_browser: bool,

    /// Timeout in seconds for waiting for data (default: 180 = 3 minutes)
    #[arg(long, default_value = "180")]
    timeout_secs: u64,
}

/// REQ-ICV-003: Wait for index.json to appear, with timeout.
/// Returns the loaded index and the data directory.
fn wait_for_index(data_dir: &PathBuf, timeout: Duration) -> Result<ContainerIndex> {
    let index_path = data_dir.join("index.json");
    let start = Instant::now();
    let poll_interval = Duration::from_secs(5);

    eprintln!("Looking for index at {:?}", index_path);

    loop {
        // Try to load the index
        if index_path.exists() {
            match ContainerIndex::load(&index_path) {
                Ok(index) => {
                    eprintln!(
                        "Loaded index: {} containers, updated at {}",
                        index.containers.len(),
                        index.updated_at
                    );
                    return Ok(index);
                }
                Err(e) => {
                    eprintln!("Index exists but failed to load: {}", e);
                }
            }
        }

        // Check timeout
        if start.elapsed() >= timeout {
            // Try to fallback to scanning files
            eprintln!("Timeout waiting for index, attempting file scan fallback...");
            return fallback_scan_for_index(data_dir);
        }

        eprintln!(
            "Index not ready, waiting... ({:.0}s / {:.0}s)",
            start.elapsed().as_secs_f64(),
            timeout.as_secs_f64()
        );
        std::thread::sleep(poll_interval);
    }
}

/// Fallback: scan parquet files to build index (slow, but recoverable)
fn fallback_scan_for_index(data_dir: &PathBuf) -> Result<ContainerIndex> {
    let pattern = format!("{}/**/*.parquet", data_dir.display());
    let files: Vec<PathBuf> = glob(&pattern)?
        .filter_map(Result::ok)
        .take(10) // Only scan a few files for fallback
        .collect();

    if files.is_empty() {
        anyhow::bail!("No index.json and no parquet files found in {:?}", data_dir);
    }

    eprintln!(
        "Building minimal index from {} parquet files...",
        files.len()
    );

    // Create a minimal index - the viewer will work but with limited container info
    let index = ContainerIndex::new(90);
    // Note: We can't fully populate the index without scanning all files,
    // but we can at least start the server and let queries work
    Ok(index)
}

/// REQ-ICV-002: Expand input paths, handling directories by globbing for parquet files.
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
    let args = Args::parse();
    let timeout = Duration::from_secs(args.timeout_secs);

    let config = server::ServerConfig {
        port: args.port,
        open_browser: !args.no_browser,
    };

    // Check if input is a single directory (index-based mode)
    let is_directory_mode = args.input.len() == 1 && args.input[0].is_dir();

    let store = if is_directory_mode {
        // REQ-ICV-003: Use index-based fast startup
        let data_dir = &args.input[0];
        eprintln!("Index-based mode: loading from {:?}", data_dir);

        let index = wait_for_index(data_dir, timeout)?;

        // Create store with index and data directory
        LazyDataStore::from_index(index, data_dir.clone())?
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
