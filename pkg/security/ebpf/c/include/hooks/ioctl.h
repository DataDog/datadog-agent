#ifndef _HOOKS_IOCTL_H
#define _HOOKS_IOCTL_H

#include "helpers/erpc.h"
#include "constants/fentry_macro.h"

HOOK_ENTRY("do_vfs_ioctl")
int hook_do_vfs_ioctl(ctx_t *ctx) {
    if (is_erpc_request(ctx)) {
        return handle_erpc_request(ctx);
    }

    return 0;
}

#endif
