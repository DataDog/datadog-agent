#ifndef BPF_TELEMETRY
#define BPF_TELEMETRY

#include <uapi/asm-generic/errno-base.h>
#include "bpf_helpers.h"
#include "map-defs.h"
#include "defs.h"

#define STR(x) #x
#define MK_MAP_KEY(map) STR(map##_telemetry_key)

#define update_indx 0
#define read_indx 1
#define read_user_indx 2
#define read_kernel_indx 3
#define MAX_ERR_TELEMETRY read_kernel_indx

#define MAX_ERRNO (ERANGE + 1)
#define ERRNO_SLOTS MAX_ERR_TELEMETRY *MAX_ERRNO

struct map_err_telemetry {
    unsigned int err_count[ERRNO_SLOTS];
};

BPF_HASH_MAP(map_err_telemetry_map, unsigned long, struct map_err_telemetry, 128)

#define helper_with_telemetry(fn, map, args...)                                                                                          \
    do {                                                                                                                                 \
        int helper_indx = -1;                                                                                                            \
        int errno_ret, errno_slot;                                                                                                       \
        errno_ret = fn(&map, args);                                                                                                      \
        if (errno_ret < 0) {                                                                                                             \
            unsigned long err_telemetry_key;                                                                                             \
            LOAD_CONSTANT(MK_MAP_KEY(map), err_telemetry_key);                                                                           \
            struct map_err_telemetry *entry =                                                                                            \
                bpf_map_lookup_elem(&map_err_telemetry_map, &err_telemetry_key);                                                         \
            if (entry) {                                                                                                                 \
                if ((unsigned long)fn == BPF_FUNC_map_update_elem) {                                                                     \
                    helper_indx = update_indx;                                                                                           \
                } else if ((unsigned long)fn == BPF_FUNC_probe_read) {                                                                   \
                    helper_indx = read_indx;                                                                                             \
                } else if (((unsigned long)fn == BPF_FUNC_probe_read_user) || ((unsigned long)fn == BPF_FUNC_probe_read_user_str)) {     \
                    helper_indx = read_user_indx;                                                                                        \
                } else if (((unsigned long)fn == BPF_FUNC_probe_read_kernel) || ((unsigned long)fn == BPF_FUNC_probe_read_kernel_str)) { \
                    helper_indx = read_kernel_indx;                                                                                      \
                }                                                                                                                        \
                if (errno_ret >= MAX_ERRNO) {                                                                                            \
                    errno_slot = MAX_ERRNO - 1;                                                                                          \
                } else {                                                                                                                 \
                    errno_slot = errno_ret - 1;                                                                                          \
                }                                                                                                                        \
                if ((helper_indx >= 0) && (errno_slot >= 0)) {                                                                           \
                    __sync_fetch_and_add(&entry->err_count[(helper_indx * MAX_ERRNO) + errno_slot], 1);                                  \
                }                                                                                                                        \
            }                                                                                                                            \
        }                                                                                                                                \
    } while (0)

#define bpf_map_update_elem(map, key, val, flags) \
    helper_with_telemetry(_bpf_map_update_elem, map, key, val, flags)

#endif
