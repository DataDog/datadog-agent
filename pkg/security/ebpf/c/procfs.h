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

SEC("kprobe/security_inode_getattr")
int kprobe_security_inode_getattr(struct pt_regs *ctx) {
    u64 pid;
    LOAD_CONSTANT("runtime_pid", pid);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    if ((u64)tgid != pid) {
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
#endif
