#ifndef __TLS_MAPS_H
#define __TLS_MAPS_H

#include "tracer.h"
#include "bpf_helpers.h"
#include "tls-types.h"

/* This map is used to keep track of in-flight TLS transactions for each TCP connection */
struct bpf_map_def SEC("maps/tls_in_flight") tls_in_flight = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(conn_tuple_t),
    .value_size = sizeof(tls_session_t),
    .max_entries = 1, // This will get overridden at runtime using max_tracked_connections
    .pinning = 0,
    .namespace = "",
};

#endif
