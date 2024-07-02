#ifndef BPF_TELEMETRY_H
#define BPF_TELEMETRY_H

#include "bpf_helpers.h"
#include "telemetry_types.h"
#include "map-defs.h"

/* redefinition of some error values */
#ifdef COMPILE_CORE
#define EEXIST 17
#define EBUSY 16
#endif

#define STR(x) #x
#define MK_KEY(key) STR(key##_telemetry_key)

BPF_HASH_MAP(map_err_telemetry_map, unsigned long, map_err_telemetry_t, 128)
BPF_HASH_MAP(helper_err_telemetry_map, unsigned long, helper_err_telemetry_t, 256)

#define PATCH_TARGET_TELEMETRY -1
static void *(*bpf_telemetry_update_patch)(unsigned long, ...) = (void *)PATCH_TARGET_TELEMETRY;

#define __record_map_telemetry(map, errno_ret) \
    long errno_slot;                                                           \
    unsigned long err_telemetry_key;                                           \
    LOAD_CONSTANT(MK_KEY(map), err_telemetry_key);                             \
    if (err_telemetry_key > 0) {                              \
        map_err_telemetry_t *entry =                                           \
        bpf_map_lookup_elem(&map_err_telemetry_map, &err_telemetry_key);       \
        if (entry) {                                                           \
            errno_slot = errno_ret * -1;                                       \
            if (errno_slot >= T_MAX_ERRNO) {                                   \
                errno_slot = T_MAX_ERRNO - 1;                                  \
                errno_slot &= (T_MAX_ERRNO - 1);                               \
            }                                                                  \
            errno_slot &= (T_MAX_ERRNO - 1);                                   \
            long *target = &entry->err_count[errno_slot];                      \
            unsigned long add = 1;                                             \
            /* Patched instruction for 4.14+: __sync_fetch_and_add(target, 1);
             * This patch point is placed here because the above instruction
             * fails on the 4.4 verifier. On 4.4 this instruction is replaced
             * with a nop: r1 = r1 */                                          \
            bpf_telemetry_update_patch((unsigned long)target, add);            \
        }                                                                      \
    }                                                                          \

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
        unsigned long telemetry_program_id;                                                     \
        LOAD_CONSTANT("telemetry_program_id_key", telemetry_program_id);                        \
        if (errno_ret < 0 && telemetry_program_id > 0) {                                        \
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
                     * to trip. Duplicating this check forces clang not to omit the check */    \
                    errno_slot &= (T_MAX_ERRNO - 1);                                            \
                }                                                                               \
                errno_slot &= (T_MAX_ERRNO - 1);                                                \
                if (helper_indx >= 0) {                                                         \
                    long *target = &entry->err_count[(helper_indx * T_MAX_ERRNO) + errno_slot]; \
                    unsigned long add = 1;                                                      \
                    /* Patched instruction for 4.14+: __sync_fetch_and_add(target, 1);
                     * This patch point is placed here because the above instruction
                     * fails on the 4.4 verifier. On 4.4 this instruction is replaced
                     * with a nop: r1 = r1 */                                                   \
                    bpf_telemetry_update_patch((unsigned long)target, add);                     \
                }                                                                               \
            }                                                                                   \
        }                                                                                       \
        errno_ret;                                                                              \
    })

#define __NEQ(one, two) ((one) != (two))
#define __AND(a, b) ((a) && (b))

#define __APPLY_OP0(...) (1)
#define __APPLY_OP1(expr, _, inv, one) expr(inv, one)
#define __APPLY_OP2(expr, op, inv, one, two) op(__APPLY_OP1(expr, op, inv, one), expr(inv, two))
#define __APPLY_OP3(expr, op, inv, one, two, three) op(__APPLY_OP2(expr, inv, one, two), expr(inv, three))

/*
 * __APPLY_OPx applies `op`, where `op` is a logical operation to all expr(inv, one),
 * expr(inv, two), etc. `inv` is the invariant and one, two, three, are the values to be
 * checked against.
 */
#define __APPLY_OPx(x,...) __APPLY_OP##x(__VA_ARGS__)

#define __SKIP_ERRS(x,...) \
    (__APPLY_OPx(x, __NEQ, __AND, __VA_ARGS__))

#define __nth(_, _1, _2, _3, N, ...) N
#define __nargs(...) __nth(_, ##__VA_ARGS__, 3, 2, 1, 0)

#define bpf_map_update_with_telemetry(map, key, val, flags,...)        \
    ({                                                                                          \
        long errno_ret = bpf_map_update_elem(&map, key, val, flags);            \
        if ((errno_ret < 0) && __SKIP_ERRS(__nargs(__VA_ARGS__), errno_ret,  __VA_ARGS__)) {                         \
            __record_map_telemetry(map, errno_ret);                                              \
        }                                                                                       \
        errno_ret;                                                                             \
    })

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
