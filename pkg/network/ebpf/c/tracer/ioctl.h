#ifndef _HOOKS_IOCTL_H
#define _HOOKS_IOCTL_H

#include "erpc.h"

#define HOOK_ENTRY(func_name) SEC("fentry/" func_name)

HOOK_ENTRY("security_file_ioctl")
int hook_security_file_ioctl(ctx_t *ctx) {
    if (is_erpc_request(ctx)) {
        return handle_erpc_request(ctx);
    }
    return 0;
}

#endif
