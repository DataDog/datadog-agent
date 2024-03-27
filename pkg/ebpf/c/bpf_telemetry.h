#ifndef BPF_TELEMETRY_H
#define BPF_TELEMETRY_H

#include "bpf_helpers.h"
#include "telemetry_types.h"
#include "map-defs.h"

// this macro reads the stack slot at offset 512
// If everything went okay, then it should have the pointer
// to the telemetry map.
#define FETCH_TELEMETRY_BLOB() ({                   \
    instrumentation_blob_t *__tb;                   \
    asm("%0 = *(u64 *)(r10 - 512)" : "=r"(__tb));   \
    __tb;                                           \
})

BPF_ARRAY_MAP(bpf_instrumentation_map, instrumentation_blob_t, 1);

#ifdef EBPF_INSTRUMENTATION

#define STR(x) #x
#define MK_MAP_INDX(key) STR(key##_telemetry_key)

#define last_writable_slot_in_blob (sizeof(instrumentation_blob_t) - 8)

#define map_update_with_telemetry(fn, map, args...)                                 \
    ({                                                                              \
        long errno_ret;                                                             \
        errno_ret = fn(&map, args);                                                 \
        unsigned long map_index;                                                    \
        LOAD_CONSTANT(MK_MAP_INDX(map), map_index);                                 \
        if (errno_ret < 0 && map_index > 0){                                        \
            instrumentation_blob_t *tb = FETCH_TELEMETRY_BLOB();                    \
            if (tb) {                                                               \
                long error = errno_ret * -1;                                        \
                long offset = (sizeof(map_err_telemetry_t) * map_index) +           \
                    (error * sizeof(unsigned long)) +                               \
                    offsetof(instrumentation_blob_t, map_err_telemetry);            \
                if (offset < last_writable_slot_in_blob) {                \
                    void *target = (void *)tb + offset;                             \
                    __sync_fetch_and_add((unsigned long *)target, 1);               \
                }                                                                   \
            }                                                                       \
        }                                                                           \
        errno_ret;                                                                  \
    })

#define MK_FN_INDX(fn) FN_INDX_##fn

#define FN_INDX_bpf_probe_read read_indx
#define FN_INDX_bpf_probe_read_kernel read_kernel_indx
#define FN_INDX_bpf_probe_read_kernel_str read_kernel_indx
#define FN_INDX_bpf_probe_read_user read_user_indx
#define FN_INDX_bpf_probe_read_user_str read_user_indx
#define FN_INDX_bpf_skb_load_bytes skb_load_bytes
#define FN_INDX_bpf_perf_event_output perf_event_output

#define helper_with_telemetry(fn, ...)                                                              \
    ({                                                                                              \
        long errno_ret = fn(__VA_ARGS__);                                                           \
        unsigned long telemetry_program_id;                                                         \
        LOAD_CONSTANT("telemetry_program_id_key", telemetry_program_id);                            \
        if (errno_ret < 0 && telemetry_program_id > 0) {                                            \
            instrumentation_blob_t *tb = FETCH_TELEMETRY_BLOB();                                    \
            if (tb) {                                                                               \
                int helper_indx = MK_FN_INDX(fn);                                                   \
                long errno_slot = errno_ret * -1;                                                   \
                long offset = (sizeof(helper_err_telemetry_t) * telemetry_program_id) +             \
                    (((helper_indx * T_MAX_ERRNO)+errno_slot)*sizeof(unsigned long)) +              \
                    offsetof(instrumentation_blob_t, helper_err_telemetry);                         \
                if ((offset > 0) && (offset < last_writable_slot_in_blob)) {                        \
                    void *target = (void *)tb + offset;                                             \
                    __sync_fetch_and_add((unsigned long *)target, 1);                               \
                }                                                                                   \
            }                                                                                       \
        }                                                                                           \
        errno_ret;                                                                                  \
    })
// If instrumentation is not enabled do not waste instructions
#else
#define map_update_with_telemetry(fn, map, args...) fn(&map, args)

#define helper_with_telemetry(fn, ...) fn(__VA_ARGS__)
#endif

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

#ifdef EBPF_INSTRUMENTATION
char _instrumentation[] SEC(".build.instrumentation") = "enabled";
#endif

#endif // BPF_TELEMETRY_H
