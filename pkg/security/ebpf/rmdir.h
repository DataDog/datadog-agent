#ifndef _RMDIR_H_
#define _RMDIR_H_

#include "syscalls.h"

struct rmdir_event_t {
    struct event_t event;
    struct process_data_t process;
    unsigned long inode;
    int mount_id;
    int overlay_numlower;
};

SYSCALL_KPROBE(rmdir) {
    struct syscall_cache_t syscall = {};
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/security_path_rmdir")
int kprobe__vfs_rmdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    // we resolve all the information before the file is actually removed
    struct path *path = (struct path *) PT_REGS_PARM1(ctx);
    struct dentry *dentry = (struct dentry *)PT_REGS_PARM2(ctx);
    struct path_key_t path_key = get_key(dentry, path);
    syscall->rmdir.path_key = path_key;
    syscall->rmdir.overlay_numlower = get_overlay_numlower(dentry);
    resolve_dentry(dentry, path_key);

    return 0;
}

SYSCALL_KRETPROBE(rmdir) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct rmdir_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_RMDIR,
        .event.timestamp = bpf_ktime_get_ns(),
        .inode = syscall->rmdir.path_key.ino,
        .mount_id = syscall->rmdir.path_key.mount_id,
        .overlay_numlower = syscall->rmdir.overlay_numlower,
    };

    fill_process_data(&event.process);

    send_event(ctx, event);

    return 0;
}

#endif
