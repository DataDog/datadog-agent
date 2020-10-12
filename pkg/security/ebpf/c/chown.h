#ifndef _CHOWN_H_
#define _CHOWN_H_

#include "syscalls.h"

struct chown_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    uid_t user;
    gid_t group;
};

int __attribute__((always_inline)) trace__sys_chown(uid_t user, gid_t group) {
    struct syscall_cache_t syscall = {
        .type = EVENT_CHOWN,
        .setattr = {
            .user = user,
            .group = group
        }
    };

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KPROBE3(lchown, const char*, filename, uid_t, user, gid_t, group) {
    return trace__sys_chown(user, group);
}

SYSCALL_KPROBE3(fchown, int, fd, uid_t, user, gid_t, group) {
    return trace__sys_chown(user, group);
}

SYSCALL_KPROBE3(chown, const char*, filename, uid_t, user, gid_t, group) {
    return trace__sys_chown(user, group);
}

SYSCALL_KPROBE3(lchown16, const char*, filename, uid_t, user, gid_t, group) {
    return trace__sys_chown(user, group);
}

SYSCALL_KPROBE3(fchown16, int, fd, uid_t, user, gid_t, group) {
    return trace__sys_chown(user, group);
}

SYSCALL_KPROBE3(chown16, const char*, filename, uid_t, user, gid_t, group) {
    return trace__sys_chown(user, group);
}

SYSCALL_KPROBE4(fchownat, int, dirfd, const char*, filename, uid_t, user, gid_t, group) {
    return trace__sys_chown(user, group);
}

int __attribute__((always_inline)) trace__sys_chown_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct chown_event_t event = {
        .event.type = EVENT_CHOWN,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .file = {
            .inode = syscall->setattr.path_key.ino,
            .mount_id = syscall->setattr.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->setattr.dentry),
        },
        .user = syscall->setattr.user,
        .group = syscall->setattr.group,
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(lchown) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(fchown) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(chown) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(lchown16) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(fchown16) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(chown16) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(fchownat) {
    return trace__sys_chown_ret(ctx);
}

#endif
