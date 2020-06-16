#ifndef _CHOWN_H_
#define _CHOWN_H_

#include "syscalls.h"

/*
  chown syscalls call notify_change that performs many checks
  which then calls security_inode_setattr
*/

struct chown_event_t {
    struct event_t event;
    struct process_data_t process;
    uid_t         user;
    gid_t         group;
    u32           padding;
    dev_t         dev;
    unsigned long inode;
};

int __attribute__((always_inline)) trace__sys_chown(struct pt_regs *ctx, uid_t user, gid_t group) {
    struct syscall_cache_t syscall = {
        .setattr = {
            .user = user,
            .group = group
        }
    };

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KPROBE(chown) {
    uid_t user;
    gid_t group;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&user, sizeof(user), &PT_REGS_PARM2(ctx));
    bpf_probe_read(&group, sizeof(group), &PT_REGS_PARM3(ctx));
#else
    user = (uid_t) PT_REGS_PARM2(ctx);
    group = (gid_t) PT_REGS_PARM3(ctx);
#endif
    return trace__sys_chown(ctx, user, group);
}

SYSCALL_KPROBE(fchown) {
    uid_t user;
    gid_t group;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&user, sizeof(user), &PT_REGS_PARM2(ctx));
    bpf_probe_read(&group, sizeof(group), &PT_REGS_PARM3(ctx));
#else
    user = (uid_t) PT_REGS_PARM2(ctx);
    group = (gid_t) PT_REGS_PARM3(ctx);
#endif
    return trace__sys_chown(ctx, user, group);
}

SYSCALL_KPROBE(fchownat) {
    uid_t user;
    gid_t group;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&user, sizeof(user), &(PT_REGS_PARM3(ctx)));
    // for some reason, this doesn't work on 5.6 kernels, so
    // we get mode from security_inode_setattr
    bpf_probe_read(&group, sizeof(group), &(PT_REGS_PARM4(ctx)));
#else
    user = (uid_t) PT_REGS_PARM3(ctx);
    group = (gid_t) PT_REGS_PARM4(ctx);
#endif
    return trace__sys_chown(ctx, user, group);
}

SYSCALL_KPROBE(lchown) {
    uid_t user;
    gid_t group;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&user, sizeof(user), &PT_REGS_PARM2(ctx));
    bpf_probe_read(&group, sizeof(group), &PT_REGS_PARM3(ctx));
#else
    user = (uid_t) PT_REGS_PARM2(ctx);
    group = (gid_t) PT_REGS_PARM3(ctx);
#endif
    return trace__sys_chown(ctx, user, group);
}

int __attribute__((always_inline)) trace__sys_chown_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct path_key_t path_key = get_dentry_key(syscall->setattr.dentry);
    struct chown_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_CHOWN,
        .event.timestamp = bpf_ktime_get_ns(),
        .user = syscall->setattr.user,
        .group = syscall->setattr.group,
        .dev = path_key.dev,
        .inode = path_key.ino,
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->setattr.dentry, path_key);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(chown) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(fchown) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(fchownat) {
    return trace__sys_chown_ret(ctx);
}

SYSCALL_KRETPROBE(lchown) {
    return trace__sys_chown_ret(ctx);
}

#endif