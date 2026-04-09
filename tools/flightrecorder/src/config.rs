use clap::Parser;

const ENV_PREFIX: &str = "DD_FLIGHTRECORDER_";

/// Flight recorder sidecar: receives signal data over a Unix socket from the
/// Datadog Agent and writes it to Parquet columnar files on disk.
#[derive(Parser, Debug, Clone)]
#[command(author, version, about, long_about = None)]
pub struct Config {
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
