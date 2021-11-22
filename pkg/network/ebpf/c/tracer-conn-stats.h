#ifndef __TRACER_CONN_STATS_H
#define __TRACER_CONN_STATS_H

#include "tracer.h"
#include "tracer-conn-maps.h"
#include "tracer-telemetry.h"

static __always_inline conn_stats_ts_t* get_conn_stats(conn_tuple_t *t) {
    // initialize-if-no-exist the connection stat, and load it
    conn_stats_ts_t empty = {};
    __builtin_memset(&empty, 0, sizeof(conn_stats_ts_t));
    if (bpf_map_update_elem(&conn_stats, t, &empty, BPF_NOEXIST) == -E2BIG) {
        increment_telemetry_count(conn_stats_max_entries_hit);
    }
    return bpf_map_lookup_elem(&conn_stats, t);
}

#endif // __TRACER_CONN_STATS_H
