#ifndef _OPEN_H_
#define _OPEN_H_

#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"

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
    struct policy_t policy = fetch_policy(EVENT_OPEN);
    if (is_discarded_by_process(policy.mode, EVENT_OPEN)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_OPEN,
        .policy = policy,
        .open = {
            .flags = flags,
            .mode = mode & S_IALLUGO,
        }
    };

    cache_syscall(&syscall);

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

struct openat2_open_how {
    u64 flags;
    u64 mode;
    u64 resolve;
};

SYSCALL_KPROBE4(openat2, int, dirfd, const char*, filename, struct openat2_open_how*, phow, size_t, size) {
    struct openat2_open_how how;
    bpf_probe_read(&how, sizeof(struct openat2_open_how), phow);
    return trace__sys_openat(how.flags, how.mode);
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

int __attribute__((always_inline)) open_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & BASENAME) > 0) {
        pass_to_userspace = approve_by_basename(syscall->open.dentry, EVENT_OPEN);
    }

    if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
        pass_to_userspace = approve_by_flags(syscall);
    }

    return pass_to_userspace;
}

int __attribute__((always_inline)) handle_open_event(struct syscall_cache_t *syscall, struct file *file, struct path *path, struct inode *inode) {
    if (syscall->open.dentry) {
        return 0;
    }

    struct dentry *dentry = get_path_dentry(path);

    syscall->open.dentry = dentry;
    syscall->open.file.path_key = get_inode_key_path(inode, path);

    set_file_inode(dentry, &syscall->open.file, 0);

    if (filter_syscall(syscall, open_approvers)) {
        return mark_as_discarded(syscall);
    }

    return 0;
}

SEC("kprobe/vfs_truncate")
int kprobe__vfs_truncate(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall)
        return 0;

    if (syscall->open.dentry) {
        return 0;
    }

    struct path *path = (struct path *)PT_REGS_PARM1(ctx);
    struct dentry *dentry = get_path_dentry(path);

    syscall->open.dentry = dentry;
    syscall->open.file.path_key = get_dentry_key_path(syscall->open.dentry, path);

    set_file_inode(dentry, &syscall->open.file, 0);

    if (filter_syscall(syscall, open_approvers)) {
        return mark_as_discarded(syscall);
    }

    return 0;
}

SEC("kprobe/vfs_open")
int kprobe__vfs_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall)
        return 0;

    struct path *path = (struct path *)PT_REGS_PARM1(ctx);
    struct file *file = (struct file *)PT_REGS_PARM2(ctx);
    struct dentry *dentry = get_path_dentry(path);
    struct inode *inode = get_dentry_inode(dentry);

    return handle_open_event(syscall, file, path, inode);
}

SEC("kprobe/do_dentry_open")
int kprobe__do_dentry_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall)
        return 0;

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct inode *inode = (struct inode *)PT_REGS_PARM2(ctx);

    return handle_exec_event(syscall, file, &file->f_path, inode);
}

struct open_flags {
    int open_flag;
    umode_t mode;
};

struct io_open {
    struct file *file;
    int dfd;
    bool ignore_nonblock;
    struct filename *filename;
    struct openat2_open_how how;
};

SEC("kprobe/io_openat2")
int kprobe__io_openat2(struct pt_regs *ctx) {
    struct io_open req;
    if (bpf_probe_read(&req, sizeof(req), (void*) PT_REGS_PARM1(ctx))) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall) {
        unsigned int flags = req.how.flags & VALID_OPEN_FLAGS;
        umode_t mode = req.how.mode & S_IALLUGO;
        return trace__sys_openat(flags, mode);
    }
    return 0;
}

int __attribute__((always_inline)) do_sys_open_ret(void *ctx, struct syscall_cache_t *syscall, int retval) {
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    // increase mount ref
    inc_mount_ref(syscall->open.file.path_key.mount_id);
    if (syscall->discarded)
        return 0;

    struct open_event_t event = {
        .syscall.retval = retval,
        .file = syscall->open.file,
        .flags = syscall->open.flags,
        .mode = syscall->open.mode,
    };

    fill_file_metadata(syscall->open.dentry, &event.file.metadata);

    int ret = resolve_dentry(syscall->open.dentry, syscall->open.file.path_key, syscall->policy.mode != NO_FILTER ? EVENT_OPEN : 0);
    if (ret == DENTRY_DISCARDED || (ret == DENTRY_INVALID && !(IS_UNHANDLED_ERROR(retval)))) {
       return 0;
    }

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    send_event(ctx, EVENT_OPEN, event);

    return 0;
}

SEC("tracepoint/handle_sys_open_exit")
int handle_sys_open_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_OPEN);
    if (!syscall)
        return 0;

    return do_sys_open_ret(args, syscall, args->ret);
}

int __attribute__((always_inline)) trace__sys_open_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_OPEN);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    return do_sys_open_ret(ctx, syscall, retval);
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

SYSCALL_KRETPROBE(openat2) {
    return trace__sys_open_ret(ctx);
}

SEC("kretprobe/io_openat2")
int kretprobe__io_openat2(struct pt_regs *ctx) {
    struct file *f = (struct file *) PT_REGS_RC(ctx);
    if (IS_ERR(f))
        return 0;

    struct syscall_cache_t *syscall = pop_syscall(EVENT_OPEN);
    if (!syscall)
        return 0;

    return do_sys_open_ret(ctx, syscall, 0);
}

SEC("kprobe/filp_close")
int kprobe__filp_close(struct pt_regs *ctx) {
    struct file *file = (struct file *) PT_REGS_PARM1(ctx);
    u32 mount_id = get_file_mount_id(file);
    if (mount_id) {
        dec_mount_ref(ctx, mount_id);
    }

    return 0;
}

#endif
