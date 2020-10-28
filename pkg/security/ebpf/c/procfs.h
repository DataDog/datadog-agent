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

SEC("kretprobe/get_task_exe_file")
int kretprobe__get_task_exe_file(struct pt_regs *ctx) {
    struct file *file = (struct file *)PT_REGS_RC(ctx);

    struct dentry *dentry = get_file_dentry(file);

    u64 inode = get_dentry_ino(dentry);
    u32 overlay_numlower = get_overlay_numlower(dentry);
    u32 mount_id = get_file_mount_id(file);

    struct inode_info_entry_t entry = {
        .mount_id = mount_id,
        .overlay_numlower = overlay_numlower,
    };

    bpf_map_update_elem(&inode_info_cache, &inode, &entry, BPF_ANY);

    return 0;
}
#endif
