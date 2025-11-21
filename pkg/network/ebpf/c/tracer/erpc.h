#ifndef _HELPERS_ERPC_H
#define _HELPERS_ERPC_H

#include "span.h"

#define CTX_PARM2(ctx) (u64)(ctx[1])
#define CTX_PARM3(ctx) (u64)(ctx[2])

#define RPC_CMD 0xdeadc001

enum DENTRY_ERPC_RESOLUTION_CODE {
    DR_ERPC_OK,
    DR_ERPC_CACHE_MISS,
    DR_ERPC_BUFFER_SIZE,
    DR_ERPC_WRITE_PAGE_FAULT,
    DR_ERPC_TAIL_CALL_ERROR,
    DR_ERPC_READ_PAGE_FAULT,
    DR_ERPC_UNKNOWN_ERROR,
};

typedef unsigned long long ctx_t;

enum erpc_op
{
    UNKNOWN_OP,
    DISCARD_INODE_OP,
    DISCARD_PID_OP, // DEPRECATED
    RESOLVE_SEGMENT_OP, // DEPRECATED
    RESOLVE_PATH_OP,
    RESOLVE_PARENT_OP, // DEPRECATED
    REGISTER_SPAN_TLS_OP, // can be used outside of the CWS, do not change the value
    EXPIRE_INODE_DISCARDER_OP,
    EXPIRE_PID_DISCARDER_OP, // DEPRECATED
    BUMP_DISCARDERS_REVISION,
    GET_RINGBUF_USAGE,
    USER_SESSION_CONTEXT_OP,
};

int __attribute__((always_inline)) is_erpc_request(ctx_t *ctx) {
    u32 cmd = CTX_PARM2(ctx);
    if (cmd != RPC_CMD) {
        return 0;
    }

    return 1;
}

int __attribute__((always_inline)) handle_erpc_request(ctx_t *ctx) {
    void *req = (void *)CTX_PARM3(ctx);

    u8 op = 0;
    int ret = bpf_probe_read(&op, sizeof(op), req);
    if (ret < 0) {
        ret = DR_ERPC_READ_PAGE_FAULT;
        return 0;
    }

    void *data = req + sizeof(op);

    switch (op) {
    case REGISTER_SPAN_TLS_OP:
        return handle_register_span_memory(data);
    }

    return 0;
}

#endif
