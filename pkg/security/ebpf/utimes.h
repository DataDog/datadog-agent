#ifndef _UTIME_H_
#define _UTIME_H_

#include "syscalls.h"

#include <uapi/linux/utime.h>

/*
  utime syscalls call notify_change that performs many checks
  which then calls security_inode_setattr
*/

struct utime_event_t {
    struct event_t event;
    struct process_data_t process;
    struct {
        long tv_sec;
        long tv_usec;
    } atime, mtime;
    u32 padding;
    dev_t         dev;
    unsigned long inode;
};

int __attribute__((always_inline)) trace__sys_utimes() {
    struct syscall_cache_t syscall = {};
    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(utime) {
    return trace__sys_utimes();
}

SYSCALL_KPROBE(utimes) {
    return trace__sys_utimes();
}

SYSCALL_KPROBE(utimensat) {
    return trace__sys_utimes();
}

SYSCALL_KPROBE(futimensat) {
    return trace__sys_utimes();
}

int __attribute__((always_inline)) trace__sys_utimes_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct path_key_t path_key = get_dentry_key(syscall->setattr.dentry);
    struct utime_event_t event = {
        .event.retval = retval,
        .event.type = EVENT_VFS_UTIME,
        .event.timestamp = bpf_ktime_get_ns(),
        .atime = {
            .tv_sec = syscall->setattr.atime.tv_sec,
            .tv_usec = syscall->setattr.atime.tv_nsec,
        },
        .mtime = {
            .tv_sec = syscall->setattr.mtime.tv_sec,
            .tv_usec = syscall->setattr.mtime.tv_nsec,
        },
        .dev = path_key.dev,
        .inode = path_key.ino,
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->setattr.dentry, path_key);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(utime) {
    return trace__sys_utimes_ret(ctx);
}

SYSCALL_KRETPROBE(utimes) {
    return trace__sys_utimes_ret(ctx);
}

SYSCALL_KRETPROBE(utimensat) {
    return trace__sys_utimes_ret(ctx);
}

SYSCALL_KRETPROBE(futimesat) {
    return trace__sys_utimes_ret(ctx);
}

#endif