#ifndef __HTTP_SHARED_MAPS_H
#define __HTTP_SHARED_MAPS_H

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

#endif //__HTTP_SHARED_MAPS_H
