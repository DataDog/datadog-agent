#ifndef _MKDIR_H_
#define _MKDIR_H_

#include "syscalls.h"

struct mkdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 mode;
    u32 padding;
};

long __attribute__((always_inline)) trace__sys_mkdir(umode_t mode) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_MKDIR,
        .mkdir = {
            .mode = mode
        }
    };

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KPROBE2(mkdir, const char*, filename, umode_t, mode)
{
    return trace__sys_mkdir(mode);
}

SYSCALL_KPROBE3(mkdirat, int, dirfd, const char*, filename, umode_t, mode)
{
    return trace__sys_mkdir(mode);
}

SEC("kprobe/vfs_mkdir")
int kprobe__security_path_mkdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_MKDIR);
    if (!syscall)
        return 0;

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM2(ctx);

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->mkdir.dentry) {
        syscall->mkdir.real_dentry = dentry;
        return 0;
    }

    syscall->mkdir.dentry = dentry;
    syscall->mkdir.path_key = get_key(syscall->mkdir.dentry, syscall->mkdir.path);

    return 0;
}

int __attribute__((always_inline)) trace__sys_mkdir_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_MKDIR);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    // the inode of the dentry was not properly set when kprobe/security_path_mkdir was called, make sur we grab it now
    syscall->mkdir.path_key.ino = get_dentry_ino(syscall->mkdir.dentry);

    resolve_dentry(syscall->mkdir.dentry, syscall->mkdir.path_key, NULL);

    // add an real entry to reach the first dentry with the proper inode
    u64 inode = syscall->mkdir.path_key.ino;
    if (syscall->mkdir.dentry) {
        inode = get_dentry_ino(syscall->mkdir.real_dentry);
        link_dentry_inode(syscall->mkdir.path_key, inode);
    }

    struct mkdir_event_t event = {
        .event.type = EVENT_MKDIR,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .file = {
            .inode = inode,
            .mount_id = syscall->mkdir.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->mkdir.dentry),
        },
        .mode = syscall->mkdir.mode,
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(mkdir)
{
    return trace__sys_mkdir_ret(ctx);
}

SYSCALL_KRETPROBE(mkdirat) {
    return trace__sys_mkdir_ret(ctx);
}

#endif
