#ifndef __CONNTRACK_MAPS_H
#define __CONNTRACK_MAPS_H

#include "bpf_helpers.h"
#include "tracer.h"
#include "conntrack-types.h"

/* This map is used for tracking conntrack entries
 */
struct bpf_map_def SEC("maps/conntrack") conntrack = {
#ifdef BPF_F_NO_COMMON_LRU
    .type = BPF_MAP_TYPE_LRU_HASH,
#else
    .type = BPF_MAP_TYPE_HASH,
#endif
    .key_size = sizeof(conn_tuple_t),
    .value_size = sizeof(conn_tuple_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

/* This map is used for conntrack telemetry in kernelspace
 * only key 0 is used
 * value is a telemetry object
 */
struct bpf_map_def SEC("maps/conntrack_telemetry") conntrack_telemetry = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(conntrack_telemetry_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

#endif
