#ifndef _ERPC_H
#define _ERPC_H

#include "filters.h"

#define RPC_CMD 0xdeadc001

enum erpc_op {
    UNKNOWN_OP,
    DISCARD_INODE_OP,
    DISCARD_PID_OP,
    RESOLVE_SEGMENT_OP,
    RESOLVE_PATH_OP,
    RESOLVE_PARENT_OP
};

int __attribute__((always_inline)) handle_discard(void *data, u64 *event_type, u64 *timeout) {
    u64 value;

    bpf_probe_read(&value, sizeof(value), data);
    *event_type = value;

    bpf_probe_read(&value, sizeof(value), data + sizeof(value));
    *timeout = value;

    return 2*sizeof(value);
}

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

struct discard_pid_t {
    struct discard_request_t req;
    u32 pid;
};

int __attribute__((always_inline)) handle_discard_inode(void *data) {
    struct discard_inode_t discarder;
    bpf_probe_read(&discarder, sizeof(discarder), data);

    return discard_inode(discarder.req.event_type, discarder.mount_id, discarder.inode, discarder.req.timeout, discarder.is_leaf);
}

int __attribute__((always_inline)) handle_discard_pid(void *data) {
    struct discard_pid_t discarder;
    bpf_probe_read(&discarder, sizeof(discarder), data);

    return discard_pid(discarder.req.event_type, discarder.pid, discarder.req.timeout);
}

int __attribute__((always_inline)) is_erpc_request(struct pt_regs *ctx) {
    u64 fd, pid;

    LOAD_CONSTANT("erpc_fd", fd);
    LOAD_CONSTANT("runtime_pid", pid);

    u32 vfs_fd = PT_REGS_PARM2(ctx);
    if (!vfs_fd || (u64)vfs_fd != fd) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    if ((u64)tgid != pid) {
        return 0;
    }

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

    if (!is_flushing_discarders()) {
        switch (op) {
            case DISCARD_INODE_OP:
                return handle_discard_inode(data);
            case DISCARD_PID_OP:
                return handle_discard_pid(data);
        }
    }

    switch (op) {
        case RESOLVE_SEGMENT_OP:
            return handle_resolve_segment(data);
        case RESOLVE_PATH_OP:
            return handle_resolve_path(ctx, data);
        case RESOLVE_PARENT_OP:
            return handle_resolve_parent(data);
    }

    return 0;
}

#endif
