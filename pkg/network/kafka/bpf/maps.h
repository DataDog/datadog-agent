// +build ignore

#pragma once

#include "defs.h"
#include "structs.h"

// Maps that cache the input arguments on the entry hook to be accessible in the return hooks.
BPF_HASH(active_connect_args_map, uint64_t, struct connect_args_t);
BPF_HASH(active_accept_args_map, uint64_t, struct accept_args_t);
BPF_HASH(active_write_args_map, uint64_t, struct data_args_t);
BPF_HASH(tls_set_fd_args_map, uint64_t, struct tls_set_fd_args_t);
BPF_HASH(tls_write_args_map, uint64_t, struct tls_data_args_t);
BPF_HASH(tls_read_args_map, uint64_t, struct tls_data_args_t);
BPF_HASH(active_read_args_map, uint64_t, struct data_args_t);
BPF_HASH(active_close_args_map, uint64_t, struct close_args_t);
BPF_HASH(active_bind_args_map, uint64_t, struct bind_args_t);

// Maps between (pid-tgid, ssl context pointer address) to fd.
BPF_HASH(tls_ctx_to_fd_map, struct tls_ctx_to_fd_key_t, int);

BPF_PERCPU_ARRAY(control_map, uint64_t, kNumProtocols);
BPF_PERCPU_ARRAY(control_values, int64_t, kNumControlValues);
BPF_PERCPU_ARRAY(socket_data_event_buffer_heap, struct socket_data_event_t, 1);

// Holds connection info that is generated in accept and connect hooks, to identify connection among other hooks.
BPF_HASH(conn_info_map, uint64_t, struct conn_info_t);

// Perf output buffers
BPF_PERF_OUTPUT(socket_data_events);
BPF_PERF_OUTPUT(socket_close_events); // Indicates a given connection was closed.
BPF_PERF_OUTPUT(malformed_socket_events); // Indicates a given connection has a malformed payload.
BPF_PERF_OUTPUT(bind_pid_events); // These output buffer get filled with pid each time a bind syscall is called
