#ifndef _ERPC_H
#define _ERPC_H

#include "filters.h"

#define RPC_CMD 0xdeadc010

enum erpc_op {
    UNKNOWN_OP,
    DISCARD_INODE_OP,
    DISCARD_PID_OP,
    RESOLVE_SEGMENT_OP,
    RESOLVE_PATH_OP
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

    bpf_printk("req disc ino: et = %llx, mi = %x, ino = %llx", discarder.req.event_type, discarder.mount_id, discarder.inode);
    bpf_printk("tmout = %llx, isleaf = %x\n", discarder.req.timeout, discarder.is_leaf);
    return discard_inode(discarder.req.event_type, discarder.mount_id, discarder.inode, discarder.req.timeout, discarder.is_leaf);
}

int __attribute__((always_inline)) handle_discard_pid(void *data) {
    struct discard_pid_t discarder;
    bpf_probe_read(&discarder, sizeof(discarder), data);

    return discard_pid(discarder.req.event_type, discarder.pid, discarder.req.timeout);
}

int __attribute__((always_inline)) is_erpc_request(u64 vfs_fd, u32 cmd, u8 *op) {
    u64 fd, pid;

    LOAD_CONSTANT("erpc_fd", fd);
    LOAD_CONSTANT("runtime_pid", pid);

    if (!vfs_fd || vfs_fd != fd) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    if ((u64)tgid != pid) {
        return 0;
    }

    if ((cmd & (~0xF)) != RPC_CMD) {
        return 0;
    }

    // extract op from cmd
    *op = cmd & 0xF;
    return 1;
}

int __attribute__((always_inline)) handle_erpc_request(struct pt_regs *ctx, u8 op, void *data) {
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
    }

    return 0;
}

int __attribute__((always_inline)) handle_erpc_request_arch_non_overlapping(struct pt_regs *ctx, u8 op, void *data) {
    if (!is_flushing_discarders()) {
        switch (op) {
            case DISCARD_INODE_OP:
                return handle_discard_inode(data);
            case DISCARD_PID_OP:
                return handle_discard_pid(data);
        }
    }

    // other operations are not supported in this fallback

    return 0;
}

#endif
