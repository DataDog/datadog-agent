#ifndef _GETATTR_H_
#define _GETATTR_H_

struct inode_info_entry_t {
    u32 mount_id;
    u32 overlay_numlower;
};

struct bpf_map_def SEC("maps/inode_info_cache") inode_info_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct inode_info_entry_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

SEC("kprobe/security_inode_getattr")
int kprobe__security_inode_getattr(struct pt_regs *ctx) {
    struct path *path = (struct path *)PT_REGS_PARM1(ctx);
    struct dentry *dentry = get_path_dentry(path);

    u64 inode = get_dentry_ino(dentry);
    u32 overlay_numlower = get_overlay_numlower(dentry);
    u32 mount_id = get_path_mount_id(path);


    struct inode_info_entry_t entry = {
        .mount_id = mount_id,
        .overlay_numlower = overlay_numlower,
    };

    // security_inode_getattr might be called multiple times on overlay filesystem, we only care about the first call
    int *current_entry = bpf_map_lookup_elem(&inode_info_cache, &inode);
    if (!current_entry) {
        bpf_map_update_elem(&inode_info_cache, &inode, &entry, BPF_ANY);
    }
    return 0;
}
#endif
