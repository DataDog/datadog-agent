use clap::Parser;

/// Flight recorder sidecar: receives signal data over a Unix socket from the
/// Datadog Agent and writes it to Parquet columnar files on disk.
#[derive(Parser, Debug, Clone)]
#[command(author, version, about, long_about = None)]
pub struct Config {
    /// Path to the Unix domain socket to listen on.
    #[arg(
        long,
        env = "RECORDER_SOCKET_PATH",
        default_value = "/var/run/flightrecorder/pipeline.sock"
    )]
    pub socket_path: String,

    /// Directory where .parquet output files are written.
    #[arg(long, env = "RECORDER_OUTPUT_DIR", default_value = "/data/signals")]
    pub output_dir: String,

    /// Number of rows to accumulate before flushing to a new Parquet file.
    #[arg(long, env = "RECORDER_FLUSH_ROWS", default_value_t = 5_000)]
    pub flush_rows: usize,

    /// Time-based flush interval in seconds.
    #[arg(long, env = "RECORDER_FLUSH_INTERVAL_SECS", default_value_t = 15)]
    pub flush_interval_secs: u64,

    /// Hours to retain old signal files before deletion.
    #[arg(long, env = "RECORDER_RETENTION_HOURS", default_value_t = 3)]
    pub retention_hours: u64,

    /// Maximum disk usage for signal files in MB. 0 = unlimited (time-based only).
    #[arg(long, env = "RECORDER_MAX_DISK_MB", default_value_t = 5120)]
    pub max_disk_mb: u64,

    /// When true, store tags inline in every metric flush file (higher RSS,
    /// self-contained files). When false (default), use a shared context file
    /// with context_key references (lower RSS, requires contexts.bin for reads).
    #[arg(long, env = "RECORDER_INLINE_CONTEXTS", default_value_t = false)]
    pub inline_contexts: bool,

    /// DogStatsD host for sidecar telemetry. Empty string disables telemetry.
    /// In Kubernetes pods, the sidecar shares the network namespace with the
    /// agent, so the default 127.0.0.1 reaches the agent's DogStatsD server.
    #[arg(long, env = "RECORDER_STATSD_HOST", default_value = "127.0.0.1")]
    pub statsd_host: String,

    /// DogStatsD port for sidecar telemetry.
    #[arg(long, env = "RECORDER_STATSD_PORT", default_value_t = 8125)]
    pub statsd_port: u16,
}
