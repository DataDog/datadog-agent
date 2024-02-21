#ifndef BPF_TELEMETRY_H
#define BPF_TELEMETRY_H

#include "bpf_helpers.h"
#include "telemetry_types.h"
#include "map-defs.h"

BPF_ARRAY_MAP(bpf_instrumentation_map, instrumentation_blob_t, 1);

#define FETCH_TELEMETRY_BLOB() ({ \
    instrumentation_blob_t *__tb; \
    asm("%0 = *(u64 *)(r10 - 512)" : "=r"(__tb)); \
    __tb; \
})

#define STR(x) #x
#define MK_MAP_INDX(key) STR(key##_telemetry_key)

#define map_update_with_telemetry(fn, map, args...)                    \
    ({                                                                 \
        long errno_ret; \
        errno_ret = fn(&map, args);                                    \
        unsigned long map_index;                                  \
        LOAD_CONSTANT(MK_MAP_INDX(map), map_index);                    \
        if (errno_ret < 0) {                                           \
            instrumentation_blob_t *tb = FETCH_TELEMETRY_BLOB();       \
            if (tb != NULL) {                                            \
                long error = errno_ret * -1;  \
                if (error >= T_MAX_ERRNO) { \
                    error = T_MAX_ERRNO - 1; \
                } \
                error &= (T_MAX_ERRNO - 1); \
                __sync_fetch_and_add(&tb->map_err_telemetry[map_index].err_count[error], 1); \
            } \
        }                                                              \
        errno_ret; \
    })

#define MK_FN_INDX(fn) FN_INDX_##fn

#define FN_INDX_bpf_probe_read read_indx
#define FN_INDX_bpf_probe_read_kernel read_kernel_indx
#define FN_INDX_bpf_probe_read_kernel_str read_kernel_indx
#define FN_INDX_bpf_probe_read_user read_user_indx
#define FN_INDX_bpf_probe_read_user_str read_user_indx
#define FN_INDX_bpf_skb_load_bytes skb_load_bytes
#define FN_INDX_bpf_perf_event_output perf_event_output

#define helper_with_telemetry(fn, ...)                                                                              \
    ({                                                                                                              \
        long errno_ret = fn(__VA_ARGS__);                                                                  \
        errno_ret;                                                                                          \
    })

#define bpf_map_update_with_telemetry(map, key, val, flags) \
    map_update_with_telemetry(bpf_map_update_elem, map, key, val, flags)

#define bpf_probe_read_with_telemetry(...) \
    helper_with_telemetry(bpf_probe_read, __VA_ARGS__)

#define bpf_probe_read_str_with_telemetry(...) \
    helper_with_telemetry(bpf_probe_read_str, __VA_ARGS__)

#define bpf_probe_read_user_with_telemetry(...) \
    helper_with_telemetry(bpf_probe_read_user, __VA_ARGS__)

#define bpf_probe_read_user_str_with_telemetry(...) \
    helper_with_telemetry(bpf_probe_read_user_str, __VA_ARGS__)

#define bpf_probe_read_kernel_with_telemetry(...) \
    helper_with_telemetry(bpf_probe_read_kernel, __VA_ARGS__)

#define bpf_probe_read_kernel_str_with_telemetry(...) \
    helper_with_telemetry(bpf_probe_read_kernel_str, __VA_ARGS__)

#define bpf_skb_load_bytes_with_telemetry(...) \
    helper_with_telemetry(bpf_skb_load_bytes, __VA_ARGS__)

#define bpf_perf_event_output_with_telemetry(...) \
    helper_with_telemetry(bpf_perf_event_output, __VA_ARGS__)

#endif // BPF_TELEMETRY_H
