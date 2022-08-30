#ifndef BPF_TELEMETRY
#define BPF_TELEMETRY

#include "bpf_helpers.h"
#include "map-defs.h"
#include "defs.h"

#define STR(x) #x
#define MK_MAP_KEY(map) STR(map##_telemetry_key)

#define update_indx 0
#define read_indx 1
#define read_user_indx 2
#define read_kernel_indx 3
#define MAX_ERR_TELEMETRY 4

struct map_err_telemetry {
    unsigned int err_count[MAX_ERR_TELEMETRY];
};

BPF_HASH_MAP(map_err_telemetry_map, unsigned long, struct map_err_telemetry, 128)

#define map_op_with_telemetry(fn, map, args...)                                                                                          \
    do {                                                                                                                                 \
        if (fn(&map, args) < 0) {                                                                                                        \
            unsigned long err_telemetry_key;                                                                                             \
            LOAD_CONSTANT(MK_MAP_KEY(map), err_telemetry_key);                                                                           \
            struct map_err_telemetry *entry =                                                                                            \
                bpf_map_lookup_elem(&map_err_telemetry_map, &err_telemetry_key);                                                         \
            if (entry) {                                                                                                                 \
                if ((unsigned long)fn == BPF_FUNC_map_update_elem) {                                                                     \
                    __sync_fetch_and_add(&entry->err_count[update_indx], 1);                                                             \
                } else if ((unsigned long)fn == BPF_FUNC_probe_read) {                                                                   \
                    __sync_fetch_and_add(&entry->err_count[read_indx], 1);                                                               \
                } else if (((unsigned long)fn == BPF_FUNC_probe_read_user) || ((unsigned long)fn == BPF_FUNC_probe_read_user_str)) {     \
                    __sync_fetch_and_add(&entry->err_count[read_user_indx], 1);                                                          \
                } else if (((unsigned long)fn == BPF_FUNC_probe_read_kernel) || ((unsigned long)fn == BPF_FUNC_probe_read_kernel_str)) { \
                    __sync_fetch_and_add(&entry->err_count[read_kernel_indx], 1);                                                        \
                }                                                                                                                        \
            }                                                                                                                            \
        }                                                                                                                                \
    } while (0)

#define bpf_map_update_elem(map, key, val, flags) \
    map_op_with_telemetry(_bpf_map_update_elem, map, key, val, flags)

#endif
