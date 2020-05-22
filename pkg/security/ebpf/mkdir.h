#ifndef _MKDIR_H_
#define _MKDIR_H_

#include "syscalls.h"

struct mkdir_event_t {
    struct event_t event;
    struct process_data_t process;
    int           mode;
    dev_t         dev;
    unsigned long inode;
};

int __attribute__((always_inline)) trace__sys_mkdir(struct pt_regs *ctx, umode_t mode) {
    if (filter_process())
        return 0;

    struct syscall_cache_t syscall = {
        .mkdir = {
            .mode = mode
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(mkdir) {
    umode_t mode;
#ifdef CONFIG_ARCH_HAS_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM2(ctx));
#else
    mode = (umode_t) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_mkdir(ctx, mode);
}

SYSCALL_KPROBE(mkdirat) {
    umode_t mode;
#ifdef CONFIG_ARCH_HAS_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM3(ctx));
#else
    mode = (umode_t) PT_REGS_PARM3(ctx);
#endif

    return trace__sys_mkdir(ctx, mode);
}

SEC("kprobe/vfs_mkdir")
int kprobe__vfs_mkdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    syscall->mkdir.dir = (struct inode *)PT_REGS_PARM1(ctx);
    syscall->mkdir.dentry = (struct dentry *)PT_REGS_PARM2(ctx);

    return 0;
}

int __attribute__((always_inline)) trace__sys_mkdir_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct path_key_t path_key = get_dentry_key(syscall->mkdir.dentry);
    struct mkdir_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_MKDIR,
        .event.timestamp = bpf_ktime_get_ns(),
        .mode = syscall->mkdir.mode,
        .dev = path_key.dev,
        .inode = path_key.ino,
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->mkdir.dentry, path_key);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(mkdir) {
    return trace__sys_mkdir_ret(ctx);
}

SYSCALL_KRETPROBE(mkdirat) {
    return trace__sys_mkdir_ret(ctx);
}

#endif