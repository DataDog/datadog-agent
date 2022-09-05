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
BPF_ARRAY_MAP(helper_err_telemetry_map, helper_err_telemetry_t, 1)

#define map_update_with_telemetry(fn, map, args...)                              \
    do {                                                                         \
        int errno_ret, errno_slot;                                               \
        errno_ret = fn(&map, args);                                              \
        if (errno_ret < 0) {                                                     \
            unsigned long err_telemetry_key;                                     \
            LOAD_CONSTANT(MK_KEY(map), err_telemetry_key);                       \
            map_err_telemetry_t *entry =                                    \
                bpf_map_lookup_elem(&map_err_telemetry_map, &err_telemetry_key); \
            if (entry) {                                                         \
                errno_slot = errno_ret * -1; \
               if (errno_slot >= T_MAX_ERRNO) { \
                   errno_slot = T_MAX_ERRNO - 1; \
               } \
               errno_slot = errno_slot & (T_MAX_ERRNO - 1); \
               __sync_fetch_and_add(&entry->err_count[errno_slot], 1);      \
            }                                                                    \
        }                                                                        \
    } while (0)

#define helper_with_telemetry(fn, errno_ret, dst, sz, src)                                              \
    do {                                                                                                \
        int helper_indx = -1;                                                                           \
        int errno_slot;                                                                                 \
        int indx = 1;                                                                                   \
        errno_ret = fn(dst, sz, src);                                                                   \
        if (errno_ret < 0) {                                                                            \
            helper_err_telemetry_t *entry =                                                        \
                bpf_map_lookup_elem(&helper_err_telemetry_map, &indx);                                  \
            if (entry) {                                                                                \
                if (IS_PROBE_READ(fn)) {                                                                \
                    helper_indx = read_indx;                                                            \
                } else if (IS_PROBE_READ_USER(fn)) {                                                    \
                    helper_indx = read_user_indx;                                                       \
                } else if (IS_PROBE_READ_KERNEL(fn)) {                                                  \
                    helper_indx = read_kernel_indx;                                                     \
                }                                                                                       \
                if (errno_ret >= T_MAX_ERRNO) {                                                           \
                    errno_slot = T_MAX_ERRNO - 1;                                                         \
                } else {                                                                                \
                    errno_slot = errno_ret - 1;                                                         \
                }                                                                                       \
                if ((helper_indx >= 0) && (errno_slot >= 0)) {                                          \
                    __sync_fetch_and_add(&entry->err_count[(helper_indx * T_MAX_ERRNO) + errno_slot], 1); \
                }                                                                                       \
            }                                                                                           \
        }                                                                                               \
    } while (0)

#define MAP_UPDATE(map, key, val, flags) \
    map_update_with_telemetry(bpf_map_update_elem, map, key, val, flags)

#define PROBE_READ(dst, sz, src, errno_ret) \
    helper_with_telemetry(bpf_probe_read, errno_ret, dst, sz, src)

#define PROBE_READ_STR(dst, sz, src, errno_ret) \
    helper_with_telemetry(bpf_probe_read_str, errno_ret, dst, sz, src)

#define PROBE_READ_USER(dst, sz, src, errno_ret) \
    helper_with_telemetry(bpf_probe_read_user, errno_ret, dst, sz, src)

#define PROBE_READ_USER_STR(dst, sz, src, errno_ret) \
    helper_with_telemetry(bpf_probe_read_user_str, errno_ret, dst, sz, src)

#define PROBE_READ_KERNEL(dst, sz, src, errno_ret) \
    helper_with_telemetry(bpf_probe_read_kernel, errno_ret, dst, sz, src)

#define PROBE_READ_KERNEL_STR(dst, sz, src, errno_ret) \
    helper_with_telemetry(bpf_probe_read_kernel_str, errno_ret, dst, sz, src)

#endif
