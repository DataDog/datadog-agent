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

SEC("kprobe/utimes_common")
int kprobe__utimes_common(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    syscall->setattr.path = (struct path *)PT_REGS_PARM1(ctx);
    syscall->setattr.dentry = get_path_dentry(syscall->setattr.path);
    return 0;
}

int __attribute__((always_inline)) trace__sys_utimes_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct path_key_t path_key = get_key(syscall->setattr.dentry, syscall->setattr.path);
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
        .mount_id = path_key.mount_id,
        .inode = path_key.ino,
        .overlay_numlower = get_overlay_numlower(syscall->setattr.dentry),
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
