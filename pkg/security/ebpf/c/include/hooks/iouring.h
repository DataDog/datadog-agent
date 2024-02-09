#ifndef _HOOKS_IOURING_H_
#define _HOOKS_IOURING_H_

#include "constants/fentry_macro.h"
#include "constants/offsets/filesystem.h"
#include "helpers/iouring.h"

SEC("tracepoint/io_uring/io_uring_create")
int io_uring_create(struct tracepoint_io_uring_io_uring_create_t *args) {
    void *ioctx = args->ctx;
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

HOOK_EXIT("io_ring_ctx_alloc")
int rethook_io_ring_ctx_alloc(ctx_t *ctx) {
    void *ioctx = (void *)CTX_PARMRET(ctx, 1);
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

HOOK_ENTRY("io_allocate_scq_urings")
int hook_io_allocate_scq_urings(ctx_t *ctx) {
    void *ioctx = (void *)CTX_PARM1(ctx);
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

HOOK_ENTRY("io_sq_offload_start")
int hook_io_sq_offload_start(ctx_t *ctx) {
    void *ioctx = (void *)CTX_PARM1(ctx);
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

#endif
