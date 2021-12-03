#ifndef __GO_TLS_MAPS_H
#define __GO_TLS_MAPS_H

#include <linux/sched.h>

#include "bpf_helpers.h"
#include "http-types.h"
#include "go-tls-types.h"

// Include the shared maps needed to enqueue http transactions
#include "http-shared-maps.h"

// Include the shared map to resolve sock structs by socket file descriptors
#include "sockfd-shared-maps.h"

/* This map passes data from user-space to the probes before they get attached */
struct bpf_map_def SEC("maps/probe_data") probe_data = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(uint32_t),
    .value_size = sizeof(tls_probe_data_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

/* This map facilitates assocating entry probe calls from return probe calls
   for the crypto/tls.(*Conn).Read function */
struct bpf_map_def SEC("maps/read_partial_calls") read_partial_calls = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(read_partial_call_key_t),
    .value_size = sizeof(read_partial_call_data_t),
    .max_entries = 1024, // TODO overwrite at runtime
    .pinning = 0,
    .namespace = "",
};

/* This map associates crypto/tls.(*Conn) values to the corresponding conn_tuple_t* value.
   It is used to implement a simplified version of tup_from_ssl_ctx from http.c */
struct bpf_map_def SEC("maps/conn_tup_by_tls_conn") conn_tup_by_tls_conn = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(uint32_t),
    .value_size = sizeof(conn_tuple_t),
    .max_entries = 1024, // TODO overwrite at runtime
    .pinning = 0,
    .namespace = "",
};

#endif //__GO_TLS_MAPS_H
