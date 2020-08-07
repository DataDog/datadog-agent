#ifndef _CHMOD_H_
#define _CHMOD_H_

#include "syscalls.h"

struct chmod_event_t {
    struct event_t event;
    struct process_data_t process;
    char container_id[CONTAINER_ID_LEN];
    int mode;
    int mount_id;
    unsigned long inode;
    int overlay_numlower;
    u32 padding;
};

int __attribute__((always_inline)) trace__sys_chmod(struct pt_regs *ctx, umode_t mode) {
    struct syscall_cache_t syscall = {
        .type = EVENT_CHMOD,
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
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM2(ctx));
#else
    mode = (umode_t) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_chmod(ctx, mode);
}

SYSCALL_KPROBE(fchmod) {
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM2(ctx));
#else
    mode = (umode_t) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_chmod(ctx, mode);
}

SYSCALL_KPROBE(fchmodat) {
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
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

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct chmod_event_t event = {
        .event.retval = retval,
        .event.type = EVENT_CHMOD,
        .event.timestamp = bpf_ktime_get_ns(),
        .mode = syscall->setattr.mode,
        .mount_id = syscall->setattr.path_key.mount_id,
        .inode = syscall->setattr.path_key.ino,
        .overlay_numlower = get_overlay_numlower(syscall->setattr.dentry),
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->setattr.dentry, syscall->setattr.path_key, NULL);

    // add process cache data
    struct proc_cache_t *entry = get_pid_cache(syscall->pid);
    if (entry) {
        copy_container_id(event.container_id, entry->container_id);
        event.process.numlower = entry->numlower;
    }

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
