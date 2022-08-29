#ifndef BPF_TELEMETRY
#define BPF_TELEMETRY

#include "bpf_helpers.h"
#include "map-defs.h"
#include "defs.h"

#define STR(x) #x
#define MK_MAP_KEY(map) STR(map##_telemetry_key)

struct map_err_telemetry {
    unsigned int err_count[5];
};


BPF_HASH_MAP(map_err_telemetry_map, unsigned long, struct map_err_telemetry, 128)

#define map_op_with_telemetry(fn, map, args...)                                 \
    do {                                                                        \
        if (fn(&map, args) < 0) {                                                \
            unsigned long err_telemetry_key;                                    \
            LOAD_CONSTANT(MK_MAP_KEY(map), err_telemetry_key);                         \
            struct map_err_telemetry* entry =                                       \
                bpf_map_lookup_elem(&map_err_telemetry_map, &err_telemetry_key);    \
            if (entry) {                                                        \
                if ((unsigned long)fn < BPF_FUNC_map_delete_elem) {             \
                    __sync_fetch_and_add(&entry->err_count[(unsigned long)fn], 1);  \
                }                                                               \
            }                                                                   \
        }                                                                       \
    } while(0) 


#define bpf_map_update_elem(map, key, val, flags) \
        map_op_with_telemetry(_bpf_map_update_elem, map, key, val, flags)

#endif
