#ifndef _RENAME_H_
#define _RENAME_H_

#include "defs.h"

struct rename_event_t {
    struct event_t event;
    struct process_data_t process;
    int    src_inode;
    u32    src_pathname_key;
    int    src_mount_id;
    int    target_inode;
    u32    target_pathname_key;
    int    target_mount_id;
};

SEC("kprobe/security_inode_rename")
int kprobe__security_inode_rename(struct pt_regs *ctx) {
    struct dentry_event_cache_t cache = {
        .src_dir = (struct inode *) PT_REGS_PARM1(ctx),
        .src_dentry = (struct dentry *) PT_REGS_PARM2(ctx),
        .target_dir = (struct inode *) PT_REGS_PARM3(ctx),
        .target_dentry = (struct dentry *) PT_REGS_PARM4(ctx),
        .flags = (int) PT_REGS_PARM5(ctx),
    };

    // Filter process
    fill_event_context(&cache.event_context);
    if (!filter(&cache.event_context))
        return 0;

    push_dentry_event_cache(&cache);

    return 0;
}

int __attribute__((always_inline)) trace__security_inode_rename_ret(struct pt_regs *ctx, int retval) {
    struct dentry_event_cache_t *cache = pop_dentry_event_cache();
    if (!cache)
        return -1;

    struct rename_event_t event = {
        .event.retval = retval,
        .event.type = EVENT_VFS_RENAME,
        .event.timestamp = bpf_ktime_get_ns(),
        .src_pathname_key = bpf_get_prandom_u32(),
        .src_inode = get_dentry_inode(cache->src_dentry),
        .src_mount_id = get_inode_mount_id(cache->src_dir),
        .target_pathname_key = bpf_get_prandom_u32(),
        .target_inode = get_dentry_inode(cache->target_dentry),
        .target_mount_id = get_inode_mount_id(cache->target_dir),
    };

    fill_process_data(&event.process);
    resolve_dentry(cache->src_dentry, event.src_pathname_key);
    resolve_dentry(cache->target_dentry, event.target_pathname_key);

    send_event(ctx, event);

    return 0;
}

SEC("kretprobe/security_inode_rename")
int kretprobe__security_inode_rename(struct pt_regs *ctx) {
    return trace__security_inode_rename_ret(ctx, PT_REGS_RC(ctx));
}

#endif