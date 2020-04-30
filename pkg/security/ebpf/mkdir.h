#ifndef _MKDIR_H
#define _MKDIR_H 1

int __attribute__((always_inline)) trace__security_inode_mkdir(struct pt_regs *ctx, struct inode *dir, struct dentry *dentry, umode_t mode) {
    struct dentry_cache_t data_cache = {};

    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);

    // Probe type
    data_cache.data.event.type = EVENT_VFS_MKDIR;

    // Add mode
    data_cache.data.mode = (int) mode;

    // Send to cache dentry
    data_cache.src_dentry = dentry;

    // Mount ID
    struct super_block *spb;
    bpf_probe_read(&spb, sizeof(spb), &dir->i_sb);

    struct list_head s_mounts;
    bpf_probe_read(&s_mounts, sizeof(s_mounts), &spb->s_mounts);

    bpf_probe_read(&data_cache.data.src_mount_id, sizeof(int), (void *) s_mounts.next + 172);

    // Filter process
    if (!filter_process(&data_cache.data.process_data))
        return 0;

    bpf_map_update_elem(&dentry_cache, &key, &data_cache, BPF_ANY);

    return 0;
}

SEC("kprobe/security_inode_mkdir")
int kprobe__security_inode_mkdir(struct pt_regs *ctx) {
    struct inode *dir = (struct inode *)PT_REGS_PARM1(ctx);
    struct dentry *dentry = (struct dentry *)PT_REGS_PARM2(ctx);
    umode_t mode = (umode_t)PT_REGS_PARM3(ctx);

    return trace__security_inode_mkdir(ctx, dir, dentry, mode);
}

int __attribute__((always_inline)) trace__security_inode_mkdir_ret(struct pt_regs *ctx, int retval) {
    u64 key = bpf_get_current_pid_tgid();

    struct dentry_cache_t *data_cache = bpf_map_lookup_elem(&dentry_cache, &key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;

    // Add inode data
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &data_cache->src_dentry);
    bpf_probe_read(&data.src_inode, sizeof(data.src_inode), &d_inode->i_ino);

    // Resolve dentry
    data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->src_dentry, data.src_pathname_key);

    data.event.retval = retval;
    data.event.timestamp = bpf_ktime_get_ns();

    u32 cpu = bpf_get_smp_processor_id();
    bpf_perf_event_output(ctx, &dentry_events, cpu, &data, sizeof(data));

    bpf_map_delete_elem(&dentry_cache, &key);

    return 0;
}

SEC("kretprobe/security_inode_mkdir")
int kretprobe__security_inode_mkdir(struct pt_regs *ctx) {
    return trace__security_inode_mkdir_ret(ctx, PT_REGS_RC(ctx));
}

#endif