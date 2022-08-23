#ifndef __IOCTL_H
#define __IOCTL_H

#include <linux/types.h>
#include "bpf_ioctl.h"

#define NPM_SIGN 0xda7ad09

static __always_inline int is_npm_request(const unsigned int cmd) {
    if (cmd != NPM_SIGN) {
        return 0;
    }
    return 1;
}

#endif
