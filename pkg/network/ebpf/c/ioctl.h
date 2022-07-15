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

static __always_inline int get_npm_request(void *req, __u32 *code, __u8 **data, __u32 *data_len) {
    struct npm_ioctl io = {};
    if(bpf_probe_read_user(&io, sizeof(io), req) < 0) {
        return -1;
    }
    *code = io.code;
    *data_len = io.data_len;
    *data = &io.data[0];
    return 0;
}

#endif
