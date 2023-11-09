#ifndef BPF_MAP_MONITORING_H
#define BPF_MAP_MONITORING_H

#include "bpf_helpers.h"
#include "map-defs.h"

#define STR(x) #x
#define MAKE_CONST_NAME(map_name) STR(map_name##_telemetry_key)

struct bpf_lru_stats_t {
    u32 hit;
    u32 miss;
};

BPF_ARRAY_MAP(bpf_lru_stats, struct bpf_lru_stats_t, 1) // max entries will be overridden at runtime

#define lru_map_lookup_with_telemetry(fn, map, key, expected)   \
    ({                                                          \
        void *ret = fn(&map, key);                              \
        u64 map_const;                                          \
        LOAD_CONSTANT(MAKE_CONST_NAME(map), map_const);         \
        u32 map_key = (u32)map_const;                           \
        struct bpf_lru_stats_t *stats =                         \
            bpf_map_lookup_elem(&bpf_lru_stats, &map_key);      \
        if (stats && !ret && expected) {                        \
            __sync_fetch_and_add(&stats->miss, 1);              \
        } else if (stats && ret) {                              \
            __sync_fetch_and_add(&stats->hit, 1);               \
        }                                                       \
        ret;                                                    \
    })

#define bpf_lru_map_lookup_elem_with_telemetry(map, key, expected) \
    lru_map_lookup_with_telemetry(bpf_map_lookup_elem, map, key, expected)

#endif // BPF_MAP_MONITORING_H
