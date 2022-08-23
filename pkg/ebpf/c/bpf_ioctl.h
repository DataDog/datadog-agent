#ifndef __BPF_IOCTL_H
#define __BPF_IOCTL_H

#include <linux/types.h>

/* The LOAD_CONSTANT macro is used to define a named constant that will be replaced
 * at runtime by the Go code. This replaces usage of a bpf_map for storing values, which
 * eliminates a bpf_map_lookup_elem per kprobe hit. The constants are best accessed with a
 * dedicated inlined function. See example functions offset_* below.
 */
#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" \
                                      : "=r"(var))

struct bpf_ioctl {
    __u64 token;
    __u32 code;
    __u32 data_len;
    __u8 data[];
};

static __always_inline bool ioctl_token_correct(struct bpf_ioctl *ioctl) {
    __u64 val = 0;
    LOAD_CONSTANT("ioctl_token_correct", val);
    return val == ioctl->token;
}

#define ioctl_get_request(type, ioctl, req) ({int __ret = 0; do {       \
                if(bpf_probe_read_user(ioctl, sizeof(type), req) < 0) { \
                    __ret = -1;                                         \
                    break;                                              \
                }                                                       \
                if(!ioctl_token_correct(ioctl)) {                       \
                    __ret = -1;                                         \
                    break;                                              \
                }                                                       \
            } while (0);                                                \
            __ret;})


#endif
