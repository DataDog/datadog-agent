use clap::Parser;

/// Pipeline recorder sidecar: receives signal data over a Unix socket from the
/// Datadog Agent and writes it to Vortex columnar files on disk.
#[derive(Parser, Debug, Clone)]
#[command(author, version, about, long_about = None)]
pub struct Config {
    /// Path to the Unix domain socket to listen on.
    #[arg(
        long,
        env = "RECORDER_SOCKET_PATH",
        default_value = "/var/run/pipelinesink/pipeline.sock"
    )]
    pub socket_path: String,

    /// Directory where .vortex output files are written.
    #[arg(long, env = "RECORDER_OUTPUT_DIR", default_value = "/data/signals")]
    pub output_dir: String,

    /// Number of rows to accumulate before flushing to a new Vortex file.
    #[arg(long, env = "RECORDER_FLUSH_ROWS", default_value_t = 10_000)]
    pub flush_rows: usize,

    /// Time-based flush interval in seconds.
    #[arg(long, env = "RECORDER_FLUSH_INTERVAL_SECS", default_value_t = 60)]
    pub flush_interval_secs: u64,

    /// Hours to retain old Vortex files before deletion.
    #[arg(long, env = "RECORDER_RETENTION_HOURS", default_value_t = 24)]
    pub retention_hours: u64,
}
