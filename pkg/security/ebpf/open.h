#ifndef _OPEN_H_
#define _OPEN_H_

struct open_event_t {
    struct event_t event;
    struct process_data_t process;
    int    mode;
    int    flags;
    int    inode;
    u32    pathname_key;
    int    mount_id;
    u32    padding;
};

int __attribute__((always_inline)) trace__security_file_open(struct pt_regs *ctx, struct file *file /*, const struct cred *cred */) {
    struct dentry_event_cache_t cache = { };

    bpf_probe_read(&cache.flags, sizeof(cache.flags), &file->f_flags);
    bpf_probe_read(&cache.mode, sizeof(cache.mode), &file->f_mode);
    bpf_probe_read(&cache.src_dir, sizeof(cache.src_dir), &file->f_inode);

    struct path path;
    bpf_probe_read(&path, sizeof(path), &file->f_path);
    cache.src_dentry = path.dentry;

    // Filter process
    fill_event_context(&cache.event_context);
    if (!filter(&cache.event_context))
        return 0;

    push_dentry_event_cache(&cache);

    return 0;
}

SEC("kprobe/security_file_open")
int kprobe__security_file_open(struct pt_regs *ctx) {
    struct file *file = (struct file *) PT_REGS_PARM1(ctx);

    return trace__security_file_open(ctx, file);
}

int __attribute__((always_inline)) trace__security_file_open_ret(struct pt_regs *ctx, int retval) {
    struct dentry_event_cache_t *cache = pop_dentry_event_cache();
    if (!cache)
        return -1;

    struct open_event_t event = {
        .event.retval = retval,
        .event.type = EVENT_MAY_OPEN,
        .event.timestamp = bpf_ktime_get_ns(),
        .mode = cache->mode,
        .flags = cache->flags,
        .pathname_key = bpf_get_prandom_u32(),
        .inode = get_dentry_inode(cache->src_dentry),
        .mount_id = get_inode_mount_id(cache->src_dir),
    };

    fill_process_data(&event.process);
    resolve_dentry(cache->src_dentry, event.pathname_key);

    send_event(ctx, event);

    return 0;
}

SEC("kretprobe/security_file_open")
int kretprobe__security_file_open(struct pt_regs *ctx) {
    return trace__security_file_open_ret(ctx, PT_REGS_RC(ctx));
}

#endif