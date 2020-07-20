#ifndef _UNLINK_H_
#define _UNLINK_H_

#include "syscalls.h"
#include "process.h"

struct unlink_event_t {
    struct event_t event;
    struct process_data_t process;
    unsigned long inode;
    int mount_id;
    int overlay_numlower;
    int flags;
    int padding;
};

int trace__sys_unlink(int flags) {
    struct syscall_cache_t syscall = {
        .type = EVENT_UNLINK,
        .unlink = {
            .flags = flags,
        }
    };
    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(unlink) {
    return trace__sys_unlink(0);
}

SYSCALL_KPROBE(unlinkat) {
    int flags;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&flags, sizeof(flags), &PT_REGS_PARM3(ctx));
#else
    flags = (int) PT_REGS_PARM3(ctx);
#endif

    return trace__sys_unlink(flags);
}

SEC("kprobe/vfs_unlink")
int kprobe__vfs_unlink(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;
    // In a container, vfs_unlink can be called multiple times to handle the different layers of the overlay filesystem.
    // The first call is the only one we really care about, the subsequent calls contain paths to the overlay work layer.
    if (syscall->unlink.path_key.ino)
        return 0;

    // we resolve all the information before the file is actually removed
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    syscall->unlink.overlay_numlower = get_overlay_numlower(dentry);
    syscall->unlink.path_key.ino = get_dentry_ino(dentry);
    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    resolve_dentry(dentry, syscall->unlink.path_key);

    return 0;
}

int __attribute__((always_inline)) trace__sys_unlink_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct unlink_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_UNLINK,
        .event.timestamp = bpf_ktime_get_ns(),
        .mount_id = syscall->unlink.path_key.mount_id,
        .inode = syscall->unlink.path_key.ino,
        .overlay_numlower = syscall->unlink.overlay_numlower,
        .flags = syscall->unlink.flags,
    };

    fill_process_data(&event.process);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(unlink) {
    return trace__sys_unlink_ret(ctx);
}

SYSCALL_KRETPROBE(unlinkat) {
    return trace__sys_unlink_ret(ctx);
}

#endif
