#ifndef __IOCTL_H
#define __IOCTL_H

#include <linux/types.h>

#define NPM_SIGN 0xda7ad09

struct npm_ioctl {
    __u32 code;
    __u32 data_len;
    __u8 data[];
};

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
    return 0;
}

#endif
