#ifndef _CHMOD_H_
#define _CHMOD_H_

#include "syscalls.h"

struct chmod_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 mode;
    u32 padding;
};

int __attribute__((always_inline)) trace__sys_chmod(umode_t mode) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_CHMOD,
        .setattr = {
            .mode = mode
        }
    };

    cache_syscall(&syscall, EVENT_CHMOD);

    if (discarded_by_process(syscall.policy.mode, EVENT_CHMOD)) {
        pop_syscall(SYSCALL_CHMOD);
    }

    return 0;
}

SYSCALL_KPROBE2(chmod, const char*, filename, umode_t, mode) {
    return trace__sys_chmod(mode);
}

SYSCALL_KPROBE2(fchmod, int, fd, umode_t, mode) {
    return trace__sys_chmod(mode);
}

SYSCALL_KPROBE3(fchmodat, int, dirfd, const char*, filename, umode_t, mode) {
    return trace__sys_chmod(mode);
}

int __attribute__((always_inline)) trace__sys_chmod_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_CHMOD);
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

    struct chmod_event_t event = {
        .event.type = EVENT_CHMOD,
        .event.timestamp = bpf_ktime_get_ns(),
        .syscall.retval = retval,
        .file = {
            .mount_id = syscall->setattr.path_key.mount_id,
            .inode = inode,
            .overlay_numlower = get_overlay_numlower(syscall->setattr.dentry),
            .path_id = syscall->setattr.path_key.path_id,
        },
        .padding = 0,
        .mode = syscall->setattr.mode,
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    // dentry resolution in setattr.h

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
