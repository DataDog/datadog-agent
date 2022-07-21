#ifndef __IOCTL_H
#define __IOCTL_H

#include <linux/types.h>

/* The LOAD_CONSTANT macro is used to define a named constant that will be replaced
 * at runtime by the Go code. This replaces usage of a bpf_map for storing values, which
 * eliminates a bpf_map_lookup_elem per kprobe hit. The constants are best accessed with a
 * dedicated inlined function. See example functions offset_* below.
 */
#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" \
                                      : "=r"(var))

#define NPM_SIGN 0xda7ad09

struct npm_ioctl {
    __u64 token;
    __u32 code;
    __u32 data_len;
    __u8 data[];
};

static __always_inline bool ioctl_token_correct(struct npm_ioctl *ioctl) {
    __u64 val = 0;
    LOAD_CONSTANT("ioctl_token_correct", val);
    return val == ioctl->token;
}

static __always_inline int is_npm_request(const unsigned int cmd) {
    if (cmd != NPM_SIGN) {
        return 0;
    }
    return 1;
}

static __always_inline int get_npm_request(struct npm_ioctl *ioctl, void *req) {
    if(bpf_probe_read_user(ioctl, sizeof(struct npm_ioctl), req) < 0) {
        return -1;
    }
    if(!ioctl_token_correct(ioctl)) {
	return -1;
    }
    return 0;
}

#endif
