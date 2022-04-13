#ifndef _PROCFS_H_
#define _PROCFS_H_

struct bpf_map_def SEC("maps/exec_file_cache") exec_file_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct file_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

__attribute__((always_inline)) int is_snapshot_process() {
    u64 pid;
    LOAD_CONSTANT("runtime_pid", pid);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    if ((u64)tgid == pid) {
        return 1;
    }
    return 0;
}

SEC("kprobe/security_inode_getattr")
int kprobe_security_inode_getattr(struct pt_regs *ctx) {
    if (!is_snapshot_process()) {
        return 0;
    }

    u32 mount_id = 0;
    struct dentry *dentry;

    u64 getattr2;
    LOAD_CONSTANT("getattr2", getattr2);

    if (getattr2) {
        struct vfsmount *mnt = (struct vfsmount *)PT_REGS_PARM1(ctx);
        mount_id = get_vfsmount_mount_id(mnt);

        dentry = (struct dentry *)PT_REGS_PARM2(ctx);
    } else {
        struct path *path = (struct path *)PT_REGS_PARM1(ctx);
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

    fill_file_metadata(dentry, &entry.metadata);

    bpf_map_update_elem(&exec_file_cache, &inode, &entry, BPF_NOEXIST);

    return 0;
}

struct bpf_map_def SEC("maps/fd_link_pid") fd_link_pid = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u8),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

SEC("kprobe/path_get")
int kprobe_path_get(struct pt_regs *ctx) {
    if (!is_snapshot_process()) {
        return 0;
    }

    // lookup the pid of the procfs path
    u8 key = 0;
    u32 *procfs_pid = bpf_map_lookup_elem(&fd_link_pid, &key);
    if (procfs_pid == NULL) {
        return 0;
    }
    u32 pid = *procfs_pid;

    struct path *p = (struct path *)PT_REGS_PARM1(ctx);
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
    bpf_map_update_elem(&flow_pid, &route, &pid, BPF_ANY);

#ifdef DEBUG
    bpf_printk("path_get netns: %u\n", route.netns);
    bpf_printk("         skc_num:%d\n", htons(route.port));
    bpf_printk("         skc_rcv_saddr:%x\n", route.addr[0]);
    bpf_printk("         pid:%d\n", pid);
#endif
    return 0;
}

SEC("kprobe/proc_fd_link")
int kprobe_proc_fd_link(struct pt_regs *ctx) {
    if (!is_snapshot_process()) {
        return 0;
    }

    struct dentry *d = (struct dentry *)PT_REGS_PARM1(ctx);
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
    bpf_printk("proc_fd_link pid:%d\n", pid);
#endif
    return 0;
}

#endif
