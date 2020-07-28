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
    struct syscall_cache_t syscall = {
        .type = EVENT_RMDIR,
    };
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/vfs_rmdir")
int kprobe__vfs_rmdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;
    // In a container, vfs_rmdir can be called multiple times to handle the different layers of the overlay filesystem.
    // The first call is the only one we really care about, the subsequent calls contain paths to the overlay work layer.
    if (syscall->rmdir.path_key.ino)
        return 0;

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM2(ctx);
    syscall->rmdir.path_key.ino = get_dentry_ino(dentry);
    syscall->rmdir.overlay_numlower = get_overlay_numlower(dentry);
    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    resolve_dentry(dentry, syscall->rmdir.path_key);

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
