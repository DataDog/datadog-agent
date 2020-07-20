#ifndef _UTIME_H_
#define _UTIME_H_

#include "syscalls.h"

#include <uapi/linux/utime.h>

/*
  utime syscalls call utimes_common
*/

struct utime_event_t {
    struct event_t event;
    struct process_data_t process;
    struct {
        long tv_sec;
        long tv_usec;
    } atime, mtime;
    u32 padding;
    int mount_id;
    unsigned long inode;
    int overlay_numlower;
    u32 padding2;
};

int __attribute__((always_inline)) trace__sys_utimes() {
    struct syscall_cache_t syscall = {
        .type = EVENT_UTIME,
    };
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

SYSCALL_KPROBE(utimesat) {
    return trace__sys_utimes();
}

SYSCALL_KPROBE(futimesat) {
    return trace__sys_utimes();
}

int __attribute__((always_inline)) trace__sys_utimes_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct utime_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_UTIME,
        .event.timestamp = bpf_ktime_get_ns(),
        .atime = {
            .tv_sec = syscall->setattr.atime.tv_sec,
            .tv_usec = syscall->setattr.atime.tv_nsec,
        },
        .mtime = {
            .tv_sec = syscall->setattr.mtime.tv_sec,
            .tv_usec = syscall->setattr.mtime.tv_nsec,
        },
        .mount_id = syscall->setattr.path_key.mount_id,
        .inode = syscall->setattr.path_key.ino,
        .overlay_numlower = get_overlay_numlower(syscall->setattr.dentry),
    };

    fill_process_data(&event.process);

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
