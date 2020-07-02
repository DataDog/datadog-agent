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
};

int trace__sys_unlink() {
    struct syscall_cache_t syscall = {};
    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(unlink) {
    return trace__sys_unlink();
}

SYSCALL_KPROBE(unlinkat) {
    return trace__sys_unlink();
}

SEC("kprobe/security_path_unlink")
int kprobe__security_path_unlink(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    // we resolve all the information before the file is actually removed
    struct path *path = (struct path *) PT_REGS_PARM1(ctx);
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    struct path_key_t path_key = get_key(dentry, path);
    syscall->unlink.path_key = path_key;
    syscall->unlink.overlay_numlower = get_overlay_numlower(dentry);
    resolve_dentry(dentry, path_key);

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
