#ifndef _RMDIR_H_
#define _RMDIR_H_

#include "syscalls.h"

struct rmdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
};

SYSCALL_KPROBE0(rmdir) {
    struct syscall_cache_t syscall = {
        .type = EVENT_RMDIR,
    };
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/security_inode_rmdir")
int kprobe__security_inode_rmdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;
    struct path_key_t key = {};
    struct dentry *dentry = NULL;
    if (syscall->type == EVENT_RMDIR) {
        dentry = (struct dentry *)PT_REGS_PARM2(ctx);
        syscall->rmdir.path_key.ino = get_dentry_ino(dentry);
        syscall->rmdir.overlay_numlower = get_overlay_numlower(dentry);
        // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
        key = syscall->rmdir.path_key;
    }
    if (syscall->type == EVENT_UNLINK) {
        // we resolve all the information before the file is actually removed
        dentry = (struct dentry *) PT_REGS_PARM2(ctx);
        syscall->unlink.overlay_numlower = get_overlay_numlower(dentry);
        syscall->unlink.path_key.ino = get_dentry_ino(dentry);
        // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
        key = syscall->unlink.path_key;
    }
    if (dentry != NULL) {
        resolve_dentry(dentry, key, NULL);
    }

    return 0;
}

SYSCALL_KRETPROBE(rmdir) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct rmdir_event_t event = {
        .event.type = EVENT_RMDIR,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .file = {
            .inode = syscall->rmdir.path_key.ino,
            .mount_id = syscall->rmdir.path_key.mount_id,
            .overlay_numlower = syscall->rmdir.overlay_numlower,
        }
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    remove_inode_discarders(&event.file);

    send_event(ctx, event);

    return 0;
}

#endif
