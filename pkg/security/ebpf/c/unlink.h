#ifndef _UNLINK_H_
#define _UNLINK_H_

#include "syscalls.h"
#include "process.h"

struct bpf_map_def SEC("maps/unlink_path_inode_discarders") unlink_path_inode_discarders = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct path_key_t),
    .value_size = sizeof(struct filter_t),
    .max_entries = 512,
    .pinning = 0,
    .namespace = "",
};

struct unlink_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 flags;
    u32 padding;
};

int __attribute__((always_inline)) trace__sys_unlink(int flags) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_UNLINK,
        .unlink = {
            .flags = flags,
        }
    };
    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE0(unlink) {
    return trace__sys_unlink(0);
}

SYSCALL_KPROBE3(unlinkat, int, dirfd, const char*, filename, int, flags) {
    return trace__sys_unlink(flags);
}

SEC("kprobe/vfs_unlink")
int kprobe__vfs_unlink(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    // we resolve all the information before the file is actually removed
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->unlink.path_key.ino) {
        syscall->unlink.real_inode = get_dentry_ino(dentry);
        return 0;
    }

    syscall->unlink.path_key.ino = get_dentry_ino(dentry);
    syscall->unlink.overlay_numlower = get_overlay_numlower(dentry);

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    int ret = 0;
    if (syscall->policy.mode == NO_FILTER) {
        ret = resolve_dentry(dentry, syscall->unlink.path_key, NULL);
    } else {
        ret = resolve_dentry(dentry, syscall->unlink.path_key, &unlink_path_inode_discarders);
    }
    if (ret < 0) {
        pop_syscall(SYSCALL_UNLINK);
    }

    return 0;
}

int __attribute__((always_inline)) trace__sys_unlink_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    // add an real entry to reach the first dentry with the proper inode
    u64 inode = syscall->unlink.path_key.ino;
    if (syscall->unlink.real_inode) {
        inode = syscall->unlink.real_inode;
        link_dentry_inode(syscall->unlink.path_key, inode);
    }

    struct unlink_event_t event = {
        .event.type = syscall->unlink.flags&AT_REMOVEDIR ? EVENT_RMDIR : EVENT_UNLINK,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .file = {
            .mount_id = syscall->unlink.path_key.mount_id,
            .inode = inode,
            .overlay_numlower = syscall->unlink.overlay_numlower,
        },
        .flags = syscall->unlink.flags,
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    remove_inode_discarders(&event.file);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(unlink) {
    return trace__sys_unlink_ret(ctx);
}

SYSCALL_KRETPROBE(unlinkat) {
    return trace__sys_unlink_ret(ctx);
}

#endif
