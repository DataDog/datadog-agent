"""
Constants used throughout the USM leak detector.
"""

# ConnTuple-keyed maps to validate (48-byte ConnTuple keys)
CONN_TUPLE_MAPS = [
    "connection_states",
    "pid_fd_by_tuple",
    "ssl_ctx_by_tuple",
    "http_in_flight",
    "redis_in_flight",
    "redis_key_in_fli",  # Truncated to 15 chars by kernel
    "postgres_in_flig",  # Truncated to 15 chars by kernel
    "http2_in_flight",
    "connection_proto",  # Truncated to 15 chars by kernel
    "tls_enhanced_tag",  # Truncated to 15 chars by kernel
]

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
