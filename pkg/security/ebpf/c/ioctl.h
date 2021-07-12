#ifndef _IOCTL_H
#define _IOCTL_H

#include "erpc.h"

SYSCALL_KPROBE3(ioctl, int, fd, unsigned int, cmd, unsigned long, arg) {
    if (is_erpc_request(fd, cmd)) {
        return handle_erpc_request(ctx, (void *)arg);
    }

    return 0;
}

#endif
