#ifndef _IOCTL_H
#define _IOCTL_H

#include "erpc.h"

SEC("kprobe/do_vfs_ioctl")
int kprobe__do_vfs_ioctl(struct pt_regs *ctx) {
    if (is_eprc_request(ctx)) {
        return handle_erpc_request(ctx);
    }

    return 0;
}

#endif
