#ifndef BPF_TELEMETRY_H
#define BPF_TELEMETRY_H

#include "bpf_helpers.h"
#include "telemetry_types.h"
#include "map-defs.h"
#include "defs.h"

#define STR(x) #x
#define MK_KEY(key) STR(key##_telemetry_key)

BPF_HASH_MAP(map_err_telemetry_map, unsigned long, map_err_telemetry_t, 128)
BPF_HASH_MAP(helper_err_telemetry_map, unsigned long, helper_err_telemetry_t, 256)

#define PATCH_TARGET_TELEMETRY -1
static void *(*bpf_telemetry_update_patch)(unsigned long, ...) = (void *)PATCH_TARGET_TELEMETRY;

#define map_update_with_telemetry(fn, map, args...)                                 \
    do {                                                                            \
        long errno_ret, errno_slot;                                                  \
        errno_ret = fn(&map, args);                                                 \
        if (errno_ret < 0) {                                                        \
            unsigned long err_telemetry_key;                                        \
            LOAD_CONSTANT(MK_KEY(map), err_telemetry_key);                          \
            map_err_telemetry_t *entry =                                            \
                bpf_map_lookup_elem(&map_err_telemetry_map, &err_telemetry_key);    \
            if (entry) {                                                            \
                errno_slot = errno_ret * -1;                                        \
                if (errno_slot >= T_MAX_ERRNO) {                                    \
                    errno_slot = T_MAX_ERRNO - 1;                                   \
                    errno_slot &= (T_MAX_ERRNO - 1);                                \
                }                                                                   \
                errno_slot &= (T_MAX_ERRNO - 1);                                    \
                long *target = &entry->err_count[errno_slot];                       \
                unsigned long add = 1;                                              \
                /* Patched instruction for 4.14+: __sync_fetch_and_add(target, 1); 
                 * This patch point is placed here because the above instruction
                 * fails on the 4.4 verifier. On 4.4 this instruction is replaced
                 * with a nop: r1 = r1 */ \
                bpf_telemetry_update_patch((unsigned long)target, add);             \
            }                                                                       \
        }                                                                           \
    } while (0)

#define MK_FN_INDX(fn) FN_INDX_##fn

#define FN_INDX_bpf_probe_read read_indx

#define FN_INDX_bpf_probe_read_kernel read_kernel_indx
#define FN_INDX_bpf_probe_read_kernel_str read_kernel_indx

#define FN_INDX_bpf_probe_read_user read_user_indx
#define FN_INDX_bpf_probe_read_user_str read_user_indx

#define FN_INDX_bpf_skb_load_bytes skb_load_bytes
#define FN_INDX_bpf_perf_event_output perf_event_output

#define helper_with_telemetry(fn, ...)                                                          \
    ({                                                                                          \
        int helper_indx = -1;                                                                   \
        long errno_slot;                                                                        \
        long errno_ret = fn(__VA_ARGS__);                                                       \
        if (errno_ret < 0) {                                                                    \
            unsigned long telemetry_program_id;                                                 \
            LOAD_CONSTANT("telemetry_program_id_key", telemetry_program_id);                    \
            helper_err_telemetry_t *entry =                                                     \
                bpf_map_lookup_elem(&helper_err_telemetry_map, &telemetry_program_id);          \
            if (entry) {                                                                        \
                helper_indx = MK_FN_INDX(fn);                                                   \
                errno_slot = errno_ret * -1;                                                    \
                if (errno_slot >= T_MAX_ERRNO) {                                                \
                    errno_slot = T_MAX_ERRNO - 1;                                               \
                    /* This is duplicated below because on clang 14.0.6 the compiler
                     * concludes that this if-check will always force errno_slot in range
                     * (0, T_MAX_ERRNO-1], and removes the bounds check, causing the verifier
                     * to trip. Duplicating this check forces clang not to omit the check */            \
                    errno_slot &= (T_MAX_ERRNO - 1);                                            \
                }                                                                               \
                errno_slot &= (T_MAX_ERRNO - 1);                                                \
                if (helper_indx >= 0) {                                                         \
                    long *target = &entry->err_count[(helper_indx * T_MAX_ERRNO) + errno_slot]; \
                    unsigned long add = 1;                                                      \
                    /* Patched instruction for 4.14+: __sync_fetch_and_add(target, 1); 
                     * This patch point is placed here because the above instruction
                     * fails on the 4.4 verifier. On 4.4 this instruction is replaced
                     * with a nop: r1 = r1 */         \
                    bpf_telemetry_update_patch((unsigned long)target, add);                     \
                }                                                                               \
            }                                                                                   \
        }                                                                                       \
        errno_ret;                                                                              \
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

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
#define bpf_skb_load_bytes_with_telemetry(...) \
    helper_with_telemetry(bpf_skb_load_bytes, __VA_ARGS__)
#endif

#define bpf_perf_event_output_with_telemetry(...) \
    helper_with_telemetry(bpf_perf_event_output, __VA_ARGS__)

#endif // BPF_TELEMETRY_H
