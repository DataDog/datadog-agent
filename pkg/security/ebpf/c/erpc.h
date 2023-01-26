#ifndef _ERPC_H
#define _ERPC_H

#include "filters.h"
#include "span.h"

#define RPC_CMD 0xdeadc001

enum erpc_op {
    UNKNOWN_OP,
    DISCARD_INODE_OP,
    DISCARD_PID_OP,
    RESOLVE_SEGMENT_OP,
    RESOLVE_PATH_OP,
    RESOLVE_PARENT_OP,
    REGISTER_SPAN_TLS_OP, // can be used outside of the CWS, do not change the value
    EXPIRE_INODE_DISCARDER_OP,
    EXPIRE_PID_DISCARDER_OP,
    BUMP_DISCARDERS_REVISION,
};

struct discard_request_t {
    u64 event_type;
    u64 timeout;
};

struct discard_inode_t {
    struct discard_request_t req;
    u64 inode;
    u32 mount_id;
    u32 is_leaf;
};

struct expire_inode_discarder_t {
    u64 inode;
    u32 mount_id;
};

struct discard_pid_t {
    struct discard_request_t req;
    u32 pid;
};

int __attribute__((always_inline)) is_runtime_request() {
    u64 pid;
    LOAD_CONSTANT("runtime_pid", pid);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    return pid_tgid >> 32 == pid;
}

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

int __attribute__((always_inline)) is_erpc_request(struct pt_regs *ctx) {
    u32 cmd = PT_REGS_PARM3(ctx);
    if (cmd != RPC_CMD) {
        return 0;
    }

    return 1;
}

int __attribute__((always_inline)) handle_erpc_request(struct pt_regs *ctx) {
    void *req = (void *)PT_REGS_PARM4(ctx);

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
        case RESOLVE_SEGMENT_OP:
            return handle_dr_request(ctx, data, DR_ERPC_SEGMENT_KEY); // func (dr *DentryResolver) ResolveFromERPC in the userspace code side triggers handle_dr_request
        case RESOLVE_PATH_OP:
            return handle_dr_request(ctx, data, DR_ERPC_KEY);
        case RESOLVE_PARENT_OP:
            return handle_dr_request(ctx, data, DR_ERPC_PARENT_KEY);
        case REGISTER_SPAN_TLS_OP:
            return handle_register_span_memory(data);
        case EXPIRE_INODE_DISCARDER_OP:
            return handle_expire_inode_discarder(data);
        case EXPIRE_PID_DISCARDER_OP:
            return handle_expire_pid_discarder(data);
        case BUMP_DISCARDERS_REVISION:
            return handle_bump_discarders_revision(data);

    }

    return 0;
}

#endif
