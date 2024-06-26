#ifndef _HELPERS_ERPC_H
#define _HELPERS_ERPC_H

#include "constants/custom.h"
#include "constants/enums.h"
#include "constants/fentry_macro.h"
#include "maps.h"
#include "perf_ring.h"

#include "discarders.h"
#include "dentry_resolver.h"
#include "span.h"
#include "user_sessions.h"

int __attribute__((always_inline)) handle_discard_inode(void *data) {
    if (!is_runtime_request()) {
        return 0;
    }

    struct discard_inode_t discarder;
    bpf_probe_read(&discarder, sizeof(discarder), data);

    return discard_inode(discarder.req.event_type, discarder.mount_id, discarder.inode, discarder.req.timeout, discarder.is_leaf);
}

int __attribute__((always_inline)) handle_expire_inode_discarder(void *data) {
    if (!is_runtime_request()) {
        return 0;
    }

    struct expire_inode_discarder_t discarder;
    bpf_probe_read(&discarder, sizeof(discarder), data);

    expire_inode_discarders(discarder.mount_id, discarder.inode);

    return 0;
}

int __attribute__((always_inline)) handle_discard_pid(void *data) {
    if (!is_runtime_request()) {
        return 0;
    }

    struct discard_pid_t discarder;
    bpf_probe_read(&discarder, sizeof(discarder), data);

    return discard_pid(discarder.req.event_type, discarder.pid, discarder.req.timeout);
}

int __attribute__((always_inline)) handle_expire_pid_discarder(void *data) {
    if (!is_runtime_request()) {
        return 0;
    }

    u32 pid;
    bpf_probe_read(&pid, sizeof(pid), data);

    expire_pid_discarder(pid);

    return 0;
}

int __attribute__((always_inline)) handle_bump_discarders_revision(void *data) {
    if (!is_runtime_request()) {
        return 0;
    }

    bump_discarders_revision();

    return 0;
}

#if USE_RING_BUFFER == 1
int __attribute__((always_inline)) handle_get_ringbuf_usage(void *data) {
    if (!is_runtime_request()) {
        return 0;
    }

    store_ring_buffer_stats();

    return 0;
}
#endif

int __attribute__((always_inline)) is_erpc_request(ctx_t *ctx) {
    u32 cmd = CTX_PARM3(ctx);
    if (cmd != RPC_CMD) {
        return 0;
    }

    return 1;
}

int __attribute__((always_inline)) handle_erpc_request(ctx_t *ctx) {
    void *req = (void *)CTX_PARM4(ctx);

    u8 op = 0;
    int ret = bpf_probe_read(&op, sizeof(op), req);
    if (ret < 0) {
        ret = DR_ERPC_READ_PAGE_FAULT;
        struct bpf_map_def *erpc_stats = select_buffer(&dr_erpc_stats_fb, &dr_erpc_stats_bb, ERPC_MONITOR_KEY);
        if (erpc_stats == NULL) {
            return 0;
        }

        struct dr_erpc_stats_t *stats = bpf_map_lookup_elem(erpc_stats, &ret);
        if (stats == NULL) {
            return 0;
        }
        __sync_fetch_and_add(&stats->count, 1);
        return 0;
    }

    void *data = req + sizeof(op);

    switch (op) {
    case DISCARD_INODE_OP:
        return handle_discard_inode(data);
    case DISCARD_PID_OP:
        return handle_discard_pid(data);
    }

    switch (op) {
    case RESOLVE_PATH_OP:
        return handle_dr_request(ctx, data, DR_ERPC_KEY);
    case USER_SESSION_CONTEXT_OP:
        return handle_register_user_session(data);
    case REGISTER_SPAN_TLS_OP:
        return handle_register_span_memory(data);
    case EXPIRE_INODE_DISCARDER_OP:
        return handle_expire_inode_discarder(data);
    case EXPIRE_PID_DISCARDER_OP:
        return handle_expire_pid_discarder(data);
    case BUMP_DISCARDERS_REVISION:
        return handle_bump_discarders_revision(data);
#if USE_RING_BUFFER == 1
    case GET_RINGBUF_USAGE:
        return handle_get_ringbuf_usage(data);
#endif
    }

    return 0;
}

#endif
