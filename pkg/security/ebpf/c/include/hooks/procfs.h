#ifndef _HOOKS_PROCFS_H_
#define _HOOKS_PROCFS_H_

#include "constants/custom.h"
#include "constants/offsets/filesystem.h"
#include "constants/offsets/netns.h"
#include "helpers/filesystem.h"
#include "helpers/utils.h"

// used during the snapshot thus this kprobe will present only at the snapshot
HOOK_ENTRY("security_inode_getattr")
int hook_security_inode_getattr(ctx_t *ctx) {
    if (!is_runtime_request()) {
        return 0;
    }

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

    u32 flags = 0;
    u64 inode = get_dentry_ino(dentry);
    if (is_overlayfs(dentry)) {
        set_overlayfs_ino(dentry, &inode, &flags);
    }

    struct file_t entry = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        },
        .flags = flags,
    };

    fill_file(dentry, &entry);

    bpf_map_update_elem(&exec_file_cache, &inode, &entry, BPF_NOEXIST);

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

    struct path *p = (struct path *)CTX_PARM1(ctx);
    struct file *sock_file = (void *)p - offsetof(struct file, f_path);
    struct pid_route_t route = {};

    struct socket *sock;
    bpf_probe_read(&sock, sizeof(sock), &sock_file->private_data);
    if (sock == NULL) {
        return 0;
    }

    struct sock *sk;
    bpf_probe_read(&sk, sizeof(sk), &sock->sk);
    if (sk == NULL) {
        return 0;
    }

    route.netns = get_netns_from_sock(sk);
    if (route.netns == 0) {
        return 0;
    }

    u16 family = 0;
    bpf_probe_read(&family, sizeof(family), &sk->__sk_common.skc_family);
    if (family == AF_INET) {
        bpf_probe_read(&route.addr, sizeof(sk->__sk_common.skc_rcv_saddr), &sk->__sk_common.skc_rcv_saddr);
    } else if (family == AF_INET6) {
        bpf_probe_read(&route.addr, sizeof(u64) * 2, &sk->__sk_common.skc_v6_rcv_saddr);
    } else {
        return 0;
    }
    bpf_probe_read(&route.port, sizeof(route.port), &sk->__sk_common.skc_num);

    // save pid route
    u32 pid = *procfs_pid;
    bpf_map_update_elem(&flow_pid, &route, &pid, BPF_ANY);

#ifdef DEBUG
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

#ifdef DEBUG
    bpf_printk("proc_fd_link pid:%d", pid);
#endif
    return 0;
}

#endif // DO_NOT_USE_TC

#endif
