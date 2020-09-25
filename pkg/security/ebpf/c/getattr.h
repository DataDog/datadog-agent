#ifndef _GETATTR_H_
#define _GETATTR_H_

struct bpf_map_def SEC("maps/inode_numlower") inode_numlower = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(int),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

SEC("kprobe/security_inode_getattr")
int kprobe__security_inode_getattr(struct pt_regs *ctx) {
    struct path *path = (struct path *)PT_REGS_PARM1(ctx);
    struct dentry *dentry = get_path_dentry(path);

    u64 inode = get_dentry_ino(dentry);
    int numlower = get_overlay_numlower(dentry);
    bpf_printk("security_inode_getattr numlower: %d, pid: %d, inode: %ld\n", numlower, (int) (bpf_get_current_pid_tgid() >> 32), inode);

    // security_inode_getattr might be called multiple times on overlay filesystem, we only care about the first call
    int *current_numlower = bpf_map_lookup_elem(&inode_numlower, &inode);
    if (!current_numlower) {
        bpf_map_update_elem(&inode_numlower, &inode, &numlower, BPF_ANY);
    }
    return 0;
}
#endif
