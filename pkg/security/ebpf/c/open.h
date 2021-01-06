#ifndef _OPEN_H_
#define _OPEN_H_
#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"
#include "open_filter.h"

struct bpf_map_def SEC("maps/open_basename_approvers") open_basename_approvers = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = BASENAME_FILTER_SIZE,
    .value_size = sizeof(u8),
    .max_entries = 255,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/open_flags_approvers") open_flags_approvers = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct open_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 flags;
    u32 mode;
};

int __attribute__((always_inline)) trace__sys_openat(int flags, umode_t mode) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_OPEN,
        .policy = {.mode = ACCEPT},
        .open = {
            .flags = flags,
            .mode = mode,
        }
    };

    cache_syscall(&syscall, EVENT_OPEN);

    if (discarded_by_process(syscall.policy.mode, EVENT_OPEN)) {
        pop_syscall(SYSCALL_OPEN);
    }

    return 0;
}

SYSCALL_KPROBE2(creat, const char *, filename, umode_t, mode) {
    int flags = O_CREAT|O_WRONLY|O_TRUNC;
    return trace__sys_openat(flags, mode);
}

SYSCALL_COMPAT_KPROBE3(open_by_handle_at, int, mount_fd, struct file_handle *, handle, int, flags) {
    umode_t mode = 0;
    return trace__sys_openat(flags, mode);
}

SYSCALL_COMPAT_KPROBE0(truncate) {
    int flags = O_CREAT|O_WRONLY|O_TRUNC;
    umode_t mode = 0;
    return trace__sys_openat(flags, mode);
}

SYSCALL_COMPAT_KPROBE3(open, const char*, filename, int, flags, umode_t, mode) {
    return trace__sys_openat(flags, mode);
}

SYSCALL_COMPAT_KPROBE4(openat, int, dirfd, const char*, filename, int, flags, umode_t, mode) {
    return trace__sys_openat(flags, mode);
}

int __attribute__((always_inline)) approve_by_basename(struct syscall_cache_t *syscall) {
    struct open_basename_t basename = {};
    get_dentry_name(syscall->open.dentry, &basename, sizeof(basename));

    struct u8 *filter = bpf_map_lookup_elem(&open_basename_approvers, &basename);
    if (filter) {
#ifdef DEBUG
        bpf_printk("open basename %s approved\n", basename.value);
#endif
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_by_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&open_flags_approvers, &key);
    if (flags != NULL && (syscall->open.flags & *flags) > 0) {
#ifdef DEBUG
        bpf_printk("open flags %d approved\n", syscall->open.flags);
#endif
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) filter_open(struct syscall_cache_t *syscall) {
    if (syscall->policy.mode == NO_FILTER)
        return 0;

    char pass_to_userspace = syscall->policy.mode == ACCEPT ? 1 : 0;

    if (syscall->policy.mode == DENY) {
        if ((syscall->policy.flags & BASENAME) > 0) {
            pass_to_userspace = approve_by_basename(syscall);
        }

        if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
           pass_to_userspace = approve_by_flags(syscall);
        }
    }

    if (!pass_to_userspace) {
        pop_syscall(SYSCALL_OPEN);
    }

    return 0;
}

int __attribute__((always_inline)) handle_open_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct inode *inode = (struct inode *)PT_REGS_PARM2(ctx);

    if (syscall->open.dentry) {
        syscall->open.real_inode = get_inode_key_path(inode, &file->f_path).ino;
        return 0;
    }

    syscall->open.dentry = get_file_dentry(file);
    syscall->open.path_key = get_inode_key_path(inode, &file->f_path);

    return filter_open(syscall);
}

SEC("kprobe/vfs_truncate")
int kprobe__vfs_truncate(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN);
    if (!syscall)
        return 0;

    struct path *path = (struct path *)PT_REGS_PARM1(ctx);

    if (syscall->open.dentry) {
        syscall->open.real_inode = get_dentry_key_path(syscall->open.dentry, path).ino;
        return 0;
    }

    syscall->open.dentry = get_path_dentry(path);
    syscall->open.path_key = get_dentry_key_path(syscall->open.dentry, path);

    return filter_open(syscall);
}

SEC("kretprobe/ovl_dentry_upper")
int kprobe__ovl_dentry_upper(struct pt_regs *ctx) {
   struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN | SYSCALL_EXEC);
    if (!syscall)
        return 0;

    struct dentry *dentry = (struct dentry *)PT_REGS_RC(ctx);
    u64 inode = get_dentry_ino(dentry);

    if (inode && !syscall->open.path_key.ino)
        syscall->open.path_key.ino = inode;

    return 0;
}

SEC("kretprobe/ovl_d_real")
int kretprobe__ovl_d_real(struct pt_regs *ctx) {
   struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN | SYSCALL_EXEC);
    if (!syscall)
        return 0;

    struct dentry *dentry = (struct dentry *)PT_REGS_RC(ctx);
    syscall->open.path_key.ino = get_dentry_ino(dentry);

    return 0;
}

SEC("kprobe/do_dentry_open")
int kprobe__do_dentry_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN | SYSCALL_EXEC);
    if (!syscall)
        return 0;

    switch(syscall->type) {
        case SYSCALL_OPEN:
            return handle_open_event(ctx, syscall);
        case SYSCALL_EXEC:
            return handle_exec_event(ctx, syscall);
    }

    return 0;
}

int __attribute__((always_inline)) trace__sys_open_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_OPEN);
    if (!syscall)
        return 0;

    syscall->open.path_key.path_id = get_path_id(0);

    // add an real entry to reach the first dentry with the proper inode
    u64 inode = syscall->open.path_key.ino;
    if (syscall->open.real_inode) {
        inode = syscall->open.real_inode;
        link_dentry_inode(syscall->open.path_key, inode);
    }

    struct open_event_t event = {
        .syscall.retval = retval,
        .file = {
            .inode = inode,
            .mount_id = syscall->open.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->open.dentry),
            .path_id = syscall->open.path_key.path_id,
        },
        .flags = syscall->open.flags,
        .mode = syscall->open.mode,
    };

    int ret = resolve_dentry(syscall->open.dentry, syscall->open.path_key, syscall->policy.mode != NO_FILTER ? EVENT_OPEN : 0);
    if (ret == DENTRY_DISCARDED || (ret == DENTRY_INVALID && !(IS_UNHANDLED_ERROR(retval)))) {
       return 0;
    }

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    send_event(ctx, EVENT_OPEN, event);

    return 0;
}

SYSCALL_KRETPROBE(creat) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(open_by_handle_at) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(truncate) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(open) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(openat) {
    return trace__sys_open_ret(ctx);
}

#endif
