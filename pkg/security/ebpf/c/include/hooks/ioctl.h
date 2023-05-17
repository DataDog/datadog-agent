#ifndef _HOOKS_IOCTL_H
#define _HOOKS_IOCTL_H

#include "helpers/erpc.h"

SEC("kprobe/do_vfs_ioctl")
int kprobe_do_vfs_ioctl(struct pt_regs *ctx) {
    if (is_erpc_request(ctx)) {
        return handle_erpc_request(ctx);
    }

    return 0;
}

#endif
