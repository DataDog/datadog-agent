"""
Constants used throughout the USM leak detector.
"""

# Default path to /proc filesystem
DEFAULT_PROC_ROOT = "/proc"

# Subprocess and timeout constants
DEFAULT_SUBPROCESS_TIMEOUT = 5  # seconds for simple commands like "which" or "version"
COMMAND_TIMEOUT = 30  # seconds for longer operations like map dumps
POLL_INTERVAL = 0.5  # seconds between subprocess polls

# Buffer sizes for streaming and I/O
STREAM_CHUNK_SIZE = 8192  # bytes for reading from streams
PIPE_READ_BUFFER_SIZE = 65536  # bytes for pipe read operations

# Report display limits
MAX_REPORT_SAMPLES = 10  # max leaked entries to show in detailed report
MAX_DEAD_PIDS_SHOWN = 20  # max dead PIDs to show in PID leak report
MAX_SAMPLES_STORED = 100  # max samples to store for analysis

# JSON parsing truncation for error messages
JSON_ERROR_PREVIEW_LENGTH = 100

# Map name prefix matching length for CLI filtering
MAP_NAME_PREFIX_LENGTH = 15  # chars to use for prefix matching truncated map names

# ConnTuple-keyed maps to validate (48-byte ConnTuple keys)
CONN_TUPLE_MAPS = [
    "connection_states",
    "pid_fd_by_tuple",
    "ssl_ctx_by_tuple",
    "http_in_flight",
    "redis_in_flight",
    "redis_key_in_fli",  # Truncated: redis_key_in_flight
    "postgres_in_flig",  # Truncated: postgres_in_flight
    "http2_in_flight",  # Key is http2_stream_key_t (52B) but ConnTuple is at offset 0
    "http2_dynamic_c",  # Truncated: http2_dynamic_counter_table
    "http2_dynamic_t",  # Truncated: http2_dynamic_table (ConnTuple at offset 8)
    "http2_incomplet",  # Truncated: http2_incomplete_frames
    "kafka_response",
    "kafka_in_flight",  # Composite key (kafka_transaction_key_t), ConnTuple at offset 0
    "go_tls_conn_by_",  # Truncated: go_tls_conn_by_tuple
    "connection_proto",  # Truncated: connection_protocol
    "tls_enhanced_tag",  # Truncated: tls_enhanced_tags
]

# ConnTuple byte offset within key (most maps have ConnTuple at offset 0)
# Only maps with non-zero offsets need entries here
CONN_TUPLE_OFFSET = {
    "http2_dynamic_t": 8,  # dynamic_table_index_t: __u64 index (8B) + conn_tuple_t
}

# PID-keyed maps to validate (uint64 pid_tgid keys)
# These maps store TLS/SSL call arguments keyed by pid_tgid.
# The PID is extracted from the upper 32 bits of the uint64 key.
PID_KEYED_MAPS = [
    "ssl_read_args",
    "ssl_read_ex_args",
    "ssl_write_args",
    "ssl_write_ex_args",
    "bio_new_socket_a",  # Truncated: bio_new_socket_args
    "ssl_ctx_by_pid_t",  # Truncated: ssl_ctx_by_pid_tgid
]

# TCP states from /proc/net/tcp (hex values)
TCP_LISTEN = 0x0A

# Recheck delay for race condition filtering
DEFAULT_RECHECK_DELAY = 2.0  # seconds

# Download timeouts
DOWNLOAD_TIMEOUT = 60  # seconds for downloading bpftool
