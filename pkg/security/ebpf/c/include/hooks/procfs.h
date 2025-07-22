#ifndef _HOOKS_PROCFS_H_
#define _HOOKS_PROCFS_H_

#include "constants/custom.h"
#include "constants/offsets/filesystem.h"
#include "constants/offsets/netns.h"
#include "constants/offsets/network.h"
#include "helpers/filesystem.h"
#include "helpers/utils.h"

static __attribute__((always_inline)) void cache_file(struct dentry *dentry, u32 mount_id) {
    u64 inode = get_dentry_ino(dentry);
    struct file_t entry = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        },
    };

    if (is_overlayfs(dentry)) {
        set_overlayfs_inode(dentry, &entry);
    }

    fill_file(dentry, &entry);

    // cache with the inode as key only as this map is used to capture the mount_id
    // the userspace as to first push an entry so that it limits to eviction caused by other stats from system-probe.
    bpf_map_update_elem(&inode_file, &entry.path_key.ino, &entry, BPF_EXIST);
}

static __attribute__((always_inline)) int handle_stat() {
    if (!is_runtime_request()) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_STAT,
    };
    cache_syscall(&syscall);
    return 0;
}

HOOK_SYSCALL_ENTRY0(newfstatat) {
    return handle_stat();
}

static __attribute__((always_inline)) int handle_ret_stat() {
    if (!is_runtime_request()) {
        return 0;
    }

    pop_syscall(EVENT_STAT);
    return 0;
}

HOOK_SYSCALL_EXIT(newfstatat) {
    return handle_ret_stat();
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_newfstatat_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return handle_ret_stat();
}

// used by both snapshot and process resolver fallback
HOOK_ENTRY("security_inode_getattr")
int hook_security_inode_getattr(ctx_t *ctx) {
    if (!is_runtime_request()) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_STAT);
    if (!syscall) {
        return 0;
    }

    if (syscall->stat.in_flight) {
        return 0;
    }
    syscall->stat.in_flight = 1;

    u32 mount_id = 0;
    struct dentry *dentry;

    u64 getattr2 = get_getattr2();

    if (getattr2) {
        struct vfsmount *mnt = (struct vfsmount *)CTX_PARM1(ctx);
        mount_id = get_vfsmount_mount_id(mnt);

        dentry = (struct dentry *)CTX_PARM2(ctx);
    } else {
        struct path *path = (struct path *)CTX_PARM1(ctx);
        mount_id = get_path_mount_id(path);

        dentry = get_path_dentry(path);
    }

    cache_file(dentry, mount_id);

    return 0;
}

#ifndef DO_NOT_USE_TC

HOOK_ENTRY("path_get")
int hook_path_get(ctx_t *ctx) {
    if (!is_runtime_request()) {
        return 0;
    }

    // lookup the pid of the procfs path
    u8 key = 0;
    u32 *procfs_pid = bpf_map_lookup_elem(&fd_link_pid, &key);
    if (procfs_pid == NULL) {
        return 0;
    }

    u64 f_path_offset;
    LOAD_CONSTANT("file_f_path_offset", f_path_offset);

    struct path *p = (struct path *)CTX_PARM1(ctx);
    struct file *sock_file = (void *)p - f_path_offset;
    struct pid_route_t route = {};
    struct pid_route_entry_t value = {};
    value.pid = *procfs_pid;
    value.type = PROCFS_ENTRY;

    struct socket *socket;
    bpf_probe_read(&socket, sizeof(socket), &sock_file->private_data);
    if (socket == NULL) {
        return 0;
    }

    struct sock *sk = get_sock_from_socket(socket);
    if (sk == NULL) {
        return 0;
    }

    route.netns = get_netns_from_sock(sk);
    if (route.netns == 0) {
        return 0;
    }

    route.port = get_skc_num_from_sock_common((void *)sk);
    if (route.port == 0) {
        // without a port we can't do much, leave early
        return 0;
    }
    route.l4_protocol = get_protocol_from_sock(sk);
    bpf_printk("procfs: l4_protocol: %u", route.l4_protocol);
    u16 family = get_family_from_sock_common((void *)sk);
    if (family == AF_INET6) {
        bpf_probe_read(&route.addr, sizeof(u64) * 2, &sk->__sk_common.skc_v6_rcv_saddr);
        bpf_map_update_elem(&flow_pid, &route, &value, BPF_ANY);

        // This AF_INET6 socket might also handle AF_INET traffic, store a mapping to AF_INET too
        family = AF_INET;
    }
    if (family == AF_INET) {
        bpf_probe_read(&route.addr, sizeof(sk->__sk_common.skc_rcv_saddr), &sk->__sk_common.skc_rcv_saddr);
        bpf_map_update_elem(&flow_pid, &route, &value, BPF_ANY);
    } else {
        // ignore unsupported traffic for now
        return 0;
    }

#if defined(DEBUG_NETNS)
    bpf_printk("path_get netns: %u", route.netns);
    bpf_printk("         skc_num:%d", htons(route.port));
    bpf_printk("         skc_rcv_saddr:%x", route.addr[0]);
    bpf_printk("         pid:%d", pid);
#endif
    return 0;
}

HOOK_ENTRY("proc_fd_link")
int hook_proc_fd_link(ctx_t *ctx) {
    if (!is_runtime_request()) {
        return 0;
    }

    struct dentry *d = (struct dentry *)CTX_PARM1(ctx);
    struct dentry *d_parent = NULL;
    struct basename_t basename = {};

    get_dentry_name(d, &basename, sizeof(basename)); // this is the file descriptor number
    bpf_probe_read(&d_parent, sizeof(d_parent), &d->d_parent);
    d = d_parent;

    get_dentry_name(d, &basename, sizeof(basename)); // this should be 'fd'
    if ((basename.value[0] != 'f') || (basename.value[1] != 'd') || (basename.value[2] != 0)) {
        return 0;
    }

    bpf_probe_read(&d_parent, sizeof(d_parent), &d->d_parent);
    d = d_parent;
    get_dentry_name(d, &basename, sizeof(basename)); // this should be the pid of the procfs path
    u32 pid = atoi(&basename.value[0]);

    u8 key = 0;
    bpf_map_update_elem(&fd_link_pid, &key, &pid, BPF_ANY);

#if defined(DEBUG_NETNS)
    bpf_printk("proc_fd_link pid:%d", pid);
#endif
    return 0;
}

#endif // DO_NOT_USE_TC

#endif
