use clap::Parser;

/// Flight recorder sidecar: receives signal data over a Unix socket from the
/// Datadog Agent and writes it to Vortex columnar files on disk.
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
    #[arg(long, env = "RECORDER_RETENTION_HOURS", default_value_t = 3)]
    pub retention_hours: u64,

    /// Maximum disk usage for Vortex files in MB. 0 = unlimited (time-based only).
    #[arg(long, env = "RECORDER_MAX_DISK_MB", default_value_t = 5120)]
    pub max_disk_mb: u64,

    /// Enable background merge/compaction of flush files. Disabled by default
    /// because decomposed-tag columns already produce compact DictArrays and
    /// the merge only achieves ~22% compression at the cost of large RSS spikes.
    #[arg(long, env = "RECORDER_MERGE_ENABLED", default_value_t = false)]
    pub merge_enabled: bool,

    /// Minimum flush files of the same type before a merge triggers.
    #[arg(long, env = "RECORDER_MERGE_MIN_FILES", default_value_t = 5)]
    pub merge_min_files: usize,

    /// Seconds between merge passes (default 300 = 5 min).
    #[arg(long, env = "RECORDER_MERGE_INTERVAL_SECS", default_value_t = 300)]
    pub merge_interval_secs: u64,
}
