#ifndef __HTTP_MAPS_H
#define __HTTP_MAPS_H

#include "tracer.h"
#include "bpf_helpers.h"
#include "http-types.h"

/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
struct bpf_map_def SEC("maps/http_in_flight") http_in_flight = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(conn_tuple_t),
    .value_size = sizeof(http_transaction_t),
    .max_entries = 1, // This will get overridden at runtime using max_tracked_connections
    .pinning = 0,
    .namespace = "",
};

/* This map used for notifying userspace that a HTTP batch is ready to be consumed */
struct bpf_map_def SEC("maps/http_notifications") http_notifications = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 0, // This will get overridden at runtime
    .pinning = 0,
    .namespace = "",
};

/* This map stores finished HTTP transactions in batches so they can be consumed by userspace*/
struct bpf_map_def SEC("maps/http_batches") http_batches = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(http_batch_key_t),
    .value_size = sizeof(http_batch_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

/* This map holds one entry per CPU storing state associated to current http batch*/
struct bpf_map_def SEC("maps/http_batch_state") http_batch_state = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u32),
    .value_size = sizeof(http_batch_state_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/ssl_sock_by_ctx") ssl_sock_by_ctx = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(void *),
    .value_size = sizeof(ssl_sock_t),
    .max_entries = 1, // This will get overridden at runtime using max_tracked_connections
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/ssl_read_args") ssl_read_args = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(ssl_read_args_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/bio_new_socket_args") bio_new_socket_args = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64), // pid_tgid
    .value_size = sizeof(__u32), // socket_fd
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/fd_by_ssl_bio") fd_by_ssl_bio = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u32),
    .value_size = sizeof(void *),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

#endif
