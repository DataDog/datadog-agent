use std::path::PathBuf;

use clap::{Parser, Subcommand};

const ENV_PREFIX: &str = "DD_FLIGHTRECORDER_";

/// Subcommands available on the flightrecorder binary.
#[derive(Subcommand, Debug, Clone)]
pub enum Commands {
    /// Convert contexts.bin to Parquet and archive all signal files as tar.zst.
    ///
    /// Reads all .parquet files from INPUT_DIR (defaulting to
    /// DD_FLIGHTRECORDER_OUTPUT_DIR / --output-dir), hydrates contexts.bin into
    /// a temporary contexts.parquet, and packs everything into a .tar.zst archive.
    Archive {
        /// Directory containing .parquet signal files and optionally contexts.bin.
        /// Defaults to --output-dir / DD_FLIGHTRECORDER_OUTPUT_DIR.
        input_dir: Option<PathBuf>,

        /// Output archive path.
        /// Defaults to signals-<timestamp_ms>.tar.zst in the current directory.
        #[arg(long, short)]
        output: Option<PathBuf>,
    },
}

/// Flight recorder sidecar: receives signal data over a Unix socket from the
/// Datadog Agent and writes it to Parquet columnar files on disk.
#[derive(Parser, Debug, Clone)]
#[command(author, version, about, long_about = None)]
pub struct Config {
    #[command(subcommand)]
    pub command: Option<Commands>,

    /// Path to the Unix domain socket to listen on.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "SOCKET_PATH"),
        default_value = "/var/run/flightrecorder/pipeline.sock")]
    pub socket_path: String,

    /// Directory where .parquet output files are written.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "OUTPUT_DIR"),
        default_value = "/data/signals")]
    pub output_dir: String,

    /// Number of rows to accumulate before flushing to a new Parquet file.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "FLUSH_ROWS"),
        default_value_t = 5_000)]
    pub flush_rows: usize,

    /// Time-based flush interval in seconds.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "FLUSH_INTERVAL_SECS"),
        default_value_t = 15)]
    pub flush_interval_secs: u64,

    /// Parquet file rotation interval in seconds. Shorter intervals reduce
    /// peak RSS (ArrowWriter accumulates less data before close) but produce
    /// more files. Default: 15 seconds.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "ROTATION_SECS"),
        default_value_t = 15)]
    pub rotation_secs: u64,

    /// Maximum disk usage for signal files in MB.
    /// The janitor deletes the oldest files when this cap is exceeded.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "MAX_DISK_MB"),
        default_value_t = 5120)]
    pub max_disk_mb: u64,

    /// DogStatsD host for sidecar telemetry. Empty string disables telemetry.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "STATSD_HOST"),
        default_value = "127.0.0.1")]
    pub statsd_host: String,

    /// DogStatsD port for sidecar telemetry.
    #[arg(long, env = const_format::concatcp!(ENV_PREFIX, "STATSD_PORT"),
        default_value_t = 8125)]
    pub statsd_port: u16,
}
