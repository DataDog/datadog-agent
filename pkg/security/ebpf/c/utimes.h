#ifndef _UTIME_H_
#define _UTIME_H_

#include "syscalls.h"

#include <uapi/linux/utime.h>

/*
  utime syscalls call utimes_common
*/

struct utime_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    struct {
        long tv_sec;
        long tv_usec;
    } atime, mtime;
};

int __attribute__((always_inline)) trace__sys_utimes() {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_UTIME,
    };

    cache_syscall(&syscall, EVENT_UTIME);

    if (discarded_by_process(syscall.policy.mode, EVENT_UTIME)) {
        pop_syscall(SYSCALL_UTIME);
    }

    return 0;
}

// On old kernels, we have sys_utime and compat_sys_utime.
// On new kernels, we have _x64_sys_utime32, __ia32_sys_utime32, __x64_sys_utime, __ia32_sys_utime
SYSCALL_COMPAT_KPROBE0(utime) {
    return trace__sys_utimes();
}

SYSCALL_KPROBE0(utime32) {
    return trace__sys_utimes();
}

SYSCALL_COMPAT_TIME_KPROBE0(utimes) {
    return trace__sys_utimes();
}

SYSCALL_COMPAT_TIME_KPROBE0(utimensat) {
    return trace__sys_utimes();
}

SYSCALL_COMPAT_TIME_KPROBE0(futimesat) {
    return trace__sys_utimes();
}

int __attribute__((always_inline)) trace__sys_utimes_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_UTIME);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    // add an real entry to reach the first dentry with the proper inode
    u64 inode = syscall->setattr.path_key.ino;
    if (syscall->setattr.real_inode) {
        inode = syscall->setattr.real_inode;
        link_dentry_inode(syscall->setattr.path_key, inode);
    }

    struct utime_event_t event = {
        .event.type = EVENT_UTIME,
        .event.timestamp = bpf_ktime_get_ns(),
        .syscall.retval = retval,
        .atime = {
            .tv_sec = syscall->setattr.atime.tv_sec,
            .tv_usec = syscall->setattr.atime.tv_nsec,
        },
        .mtime = {
            .tv_sec = syscall->setattr.mtime.tv_sec,
            .tv_usec = syscall->setattr.mtime.tv_nsec,
        },
        .file = {
            .inode = inode,
            .mount_id = syscall->setattr.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->setattr.dentry),
            .path_id = syscall->setattr.path_key.path_id,
        },
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    // dentry resolution in setattr.h

    send_event(ctx, event);

    return 0;
}

SYSCALL_COMPAT_KRETPROBE(utime) {
    return trace__sys_utimes_ret(ctx);
}

SYSCALL_KRETPROBE(utime32) {
    return trace__sys_utimes_ret(ctx);
}

SYSCALL_COMPAT_TIME_KRETPROBE(utimes) {
    return trace__sys_utimes_ret(ctx);
}

SYSCALL_COMPAT_TIME_KRETPROBE(utimensat) {
    return trace__sys_utimes_ret(ctx);
}

SYSCALL_COMPAT_TIME_KRETPROBE(futimesat) {
    return trace__sys_utimes_ret(ctx);
}

#endif
