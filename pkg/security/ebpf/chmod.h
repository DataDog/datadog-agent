#ifndef _CHMOD_H_
#define _CHMOD_H_

#include "syscalls.h"

/*
  chmod syscalls call notify_change that performs many checks
  which then calls security_inode_setattr
*/

struct chmod_event_t {
    struct event_t event;
    struct process_data_t process;
    int           mode;
    dev_t         dev;
    unsigned long inode;
};

int __attribute__((always_inline)) trace__sys_chmod(struct pt_regs *ctx, umode_t mode) {
    struct syscall_cache_t syscall = {
        .setattr = {
            .mode = mode
        }
    };

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KPROBE(chmod) {
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM2(ctx));
#else
    mode = (umode_t) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_chmod(ctx, mode);
}

SYSCALL_KPROBE(fchmod) {
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM2(ctx));
#else
    mode = (umode_t) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_chmod(ctx, mode);
}

SYSCALL_KPROBE(fchmodat) {
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM3(ctx));
#else
    mode = (umode_t) PT_REGS_PARM3(ctx);
#endif

    return trace__sys_chmod(ctx, mode);
}

int __attribute__((always_inline)) trace__sys_chmod_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct path_key_t path_key = get_dentry_key(syscall->setattr.dentry);
    struct chmod_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_CHMOD,
        .event.timestamp = bpf_ktime_get_ns(),
        .mode = syscall->setattr.mode,
        .dev = path_key.dev,
        .inode = path_key.ino,
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->setattr.dentry, path_key);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(chmod) {
    return trace__sys_chmod_ret(ctx);
}

SYSCALL_KRETPROBE(fchmod) {
    return trace__sys_chmod_ret(ctx);
}

SYSCALL_KRETPROBE(fchmodat) {
    return trace__sys_chmod_ret(ctx);
}

#endif