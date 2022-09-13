#ifndef BPF_TELEMETRY_H
#define BPF_TELEMETRY_H

#include "bpf_helpers.h"
#include "telemetry_types.h"
#include "map-defs.h"
#include "defs.h"

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 11, 0) || RHEL_MAJOR == 7
#define IS_PROBE_READ(fn) \
    ((((unsigned long)fn) == BPF_FUNC_probe_read) || (((unsigned long)fn) == BPF_FUNC_probe_read_str))
#else
#define IS_PROBE_READ(fn) \
    (((unsigned long)fn) == BPF_FUNC_probe_read)
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(5, 5, 0)
#define IS_PROBE_READ_USER_STR(fn) (((unsigned long)fn) == (BPF_FUNC_probe_read_user_str))
#define IS_PROBE_READ_USER(fn) (IS_PROBE_READ_USER_STR(fn) || (((unsigned long)fn) == BPF_FUNC_probe_read_user))
#define IS_PROBE_READ_KERNEL_STR(fn) (((unsigned long)fn) == (BPF_FUNC_probe_read_kernel_str))
#define IS_PROBE_READ_KERNEL(fn) (IS_PROBE_READ_KERNEL_STR(fn) || (((unsigned long)fn) == BPF_FUNC_probe_read_kernel))
#else
#define IS_PROBE_READ_USER(fn) 0
#define IS_PROBE_READ_KERNEL(fn) 0
#endif

#define STR(x) #x
#define MK_KEY(key) STR(key##_telemetry_key)

BPF_HASH_MAP(map_err_telemetry_map, unsigned long, map_err_telemetry_t, 128)
BPF_HASH_MAP(helper_err_telemetry_map, unsigned long, helper_err_telemetry_t, 256)

#define PATCH_TARGET_TELEMETRY -1
static void *(*bpf_telemetry_update_patch)(unsigned long, ...) = (void *)PATCH_TARGET_TELEMETRY;

#define map_update_with_telemetry(fn, map, args...)                                 \
    do {                                                                            \
        int errno_ret, errno_slot;                                                  \
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
                }                                                                   \
                errno_slot &= (T_MAX_ERRNO - 1);                                    \
                int *target = &entry->err_count[errno_slot];                        \
                unsigned long add = 1;                                              \
                /* Patched instruction for 4.14+: __sync_fetch_and_add(target, 1); 
                 * This patch point is placed here because the above instruction
                 * fails on the 4.4 verifier. */ \
                bpf_telemetry_update_patch((unsigned long)target, add);             \
            }                                                                       \
        }                                                                           \
    } while (0)

#define helper_with_telemetry(fn, dst, sz, src)                                                \
    ({                                                                                         \
        int helper_indx = -1;                                                                  \
        int errno_slot;                                                                        \
        int errno_ret = fn(dst, sz, src);                                                      \
        if (errno_ret < 0) {                                                                   \
            unsigned long telemetry_program_id;                                                \
            LOAD_CONSTANT("telemetry_program_id_key", telemetry_program_id);                   \
            helper_err_telemetry_t *entry =                                                    \
                bpf_map_lookup_elem(&helper_err_telemetry_map, &telemetry_program_id);         \
            if (entry) {                                                                       \
                if (IS_PROBE_READ(fn)) {                                                       \
                    helper_indx = read_indx;                                                   \
                } else if (IS_PROBE_READ_USER(fn)) {                                           \
                    helper_indx = read_user_indx;                                              \
                } else if (IS_PROBE_READ_KERNEL(fn)) {                                         \
                    helper_indx = read_kernel_indx;                                            \
                }                                                                              \
                errno_slot = errno_ret * -1;                                                   \
                if (errno_slot >= T_MAX_ERRNO) {                                               \
                    errno_slot = T_MAX_ERRNO - 1;                                              \
                }                                                                              \
                errno_slot &= (T_MAX_ERRNO - 1);                                               \
                if (helper_indx >= 0) {                                                        \
                    int *target = &entry->err_count[(helper_indx * T_MAX_ERRNO) + errno_slot]; \
                    unsigned long add = 1;                                                     \
                    /* Patched instruction for 4.14+: __sync_fetch_and_add(target, 1); */      \
                    bpf_telemetry_update_patch((unsigned long)target, add);                    \
                }                                                                              \
            }                                                                                  \
        }                                                                                      \
        errno_ret;                                                                             \
    })

#define bpf_map_update_with_telemetry(map, key, val, flags) \
    map_update_with_telemetry(bpf_map_update_elem, map, key, val, flags)

#define bpf_probe_read_with_telemetry(dst, sz, src) \
    helper_with_telemetry(bpf_probe_read, dst, sz, src)

#define bpf_probe_read_str_with_telemetry(dst, sz, src) \
    helper_with_telemetry(bpf_probe_read_str, dst, sz, src)

#define bpf_probe_read_user_with_telemetry(dst, sz, src) \
    helper_with_telemetry(bpf_probe_read_user, dst, sz, src)

#define bpf_probe_read_user_str_with_telemetry(dst, sz, src) \
    helper_with_telemetry(bpf_probe_read_user_str, dst, sz, src)

#define bpf_probe_read_kernel_with_telemetry(dst, sz, src) \
    helper_with_telemetry(bpf_probe_read_kernel, dst, sz, src)

#define bpf_probe_read_kernel_str_with_telemetry(dst, sz, src) \
    helper_with_telemetry(bpf_probe_read_kernel_str, dst, sz, src)

#endif // BPF_TELEMETRY_H
