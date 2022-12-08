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
};

struct open_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 flags;
    u32 mode;
};

int __attribute__((always_inline)) trace__sys_openat2(u8 async, int flags, umode_t mode, u64 pid_tgid) {
    struct policy_t policy = fetch_policy(EVENT_OPEN);
    if (is_discarded_by_process(policy.mode, EVENT_OPEN)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_OPEN,
        .policy = policy,
        .async = async,
        .open = {
            .flags = flags,
            .mode = mode & S_IALLUGO,
        }
    };

    if (pid_tgid > 0) {
        syscall.open.pid_tgid = pid_tgid;
    }

    cache_syscall(&syscall);

    return 0;
}

int __attribute__((always_inline)) trace__sys_openat(u8 async, int flags, umode_t mode) {
    return trace__sys_openat2(async, flags, mode, 0);
}

SYSCALL_KPROBE2(creat, const char *, filename, umode_t, mode) {
    int flags = O_CREAT|O_WRONLY|O_TRUNC;
    return trace__sys_openat(SYNC_SYSCALL, flags, mode);
}

SYSCALL_COMPAT_KPROBE3(open_by_handle_at, int, mount_fd, struct file_handle *, handle, int, flags) {
    umode_t mode = 0;
    return trace__sys_openat(SYNC_SYSCALL, flags, mode);
}

SYSCALL_COMPAT_KPROBE0(truncate) {
    int flags = O_CREAT|O_WRONLY|O_TRUNC;
    umode_t mode = 0;
    return trace__sys_openat(SYNC_SYSCALL, flags, mode);
}

SYSCALL_COMPAT_KPROBE3(open, const char*, filename, int, flags, umode_t, mode) {
    return trace__sys_openat(SYNC_SYSCALL, flags, mode);
}

SYSCALL_COMPAT_KPROBE4(openat, int, dirfd, const char*, filename, int, flags, umode_t, mode) {
    return trace__sys_openat(SYNC_SYSCALL, flags, mode);
}

struct openat2_open_how {
    u64 flags;
    u64 mode;
    u64 resolve;
};

SYSCALL_KPROBE4(openat2, int, dirfd, const char*, filename, struct openat2_open_how*, phow, size_t, size) {
    struct openat2_open_how how;
    bpf_probe_read(&how, sizeof(struct openat2_open_how), phow);
    return trace__sys_openat(SYNC_SYSCALL, how.flags, how.mode);
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
int kprobe_vfs_truncate(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall) {
        return 0;
    }

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
int kprobe_vfs_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall) {
        return 0;
    }

    struct path *path = (struct path *)PT_REGS_PARM1(ctx);
    struct file *file = (struct file *)PT_REGS_PARM2(ctx);
    struct dentry *dentry = get_path_dentry(path);
    struct inode *inode = get_dentry_inode(dentry);

    return handle_open_event(syscall, file, path, inode);
}

SEC("kprobe/do_dentry_open")
int kprobe_do_dentry_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        return 0;
    }

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct inode *inode = (struct inode *)PT_REGS_PARM2(ctx);

    return handle_exec_event(ctx, syscall, file, &file->f_path, inode);
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

#ifndef VALID_OPEN_FLAGS
#define VALID_OPEN_FLAGS \
        (O_RDONLY | O_WRONLY | O_RDWR | O_CREAT | O_EXCL | O_NOCTTY | O_TRUNC | \
         O_APPEND | O_NDELAY | O_NONBLOCK | __O_SYNC | O_DSYNC | \
         FASYNC | O_DIRECT | O_LARGEFILE | O_DIRECTORY | O_NOFOLLOW | \
         O_NOATIME | O_CLOEXEC | O_PATH | __O_TMPFILE)
#endif

int __attribute__((always_inline)) trace_io_openat(struct pt_regs *ctx) {
    void *raw_req = (void *)PT_REGS_PARM1(ctx);

    struct io_open req;
    if (bpf_probe_read(&req, sizeof(req), raw_req)) {
        return 0;
    }

    u64 pid_tgid = get_pid_tgid_from_iouring(raw_req);

    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall) {
        unsigned int flags = req.how.flags & VALID_OPEN_FLAGS;
        umode_t mode = req.how.mode & S_IALLUGO;
        return trace__sys_openat2(ASYNC_SYSCALL, flags, mode, pid_tgid);
    } else {
        syscall->open.pid_tgid = get_pid_tgid_from_iouring(raw_req);
    }
    return 0;
}

SEC("kprobe/io_openat")
int kprobe_io_openat(struct pt_regs *ctx) {
    return trace_io_openat(ctx);
}

SEC("kprobe/io_openat2")
int kprobe_io_openat2(struct pt_regs *ctx) {
    return trace_io_openat(ctx);
}

int __attribute__((always_inline)) sys_open_ret(void *ctx, int retval, int dr_type) {
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall) {
        return 0;
    }

    // increase mount ref
    inc_mount_ref(syscall->open.file.path_key.mount_id);
    if (syscall->discarded) {
        return 0;
    }

    syscall->resolver.key = syscall->open.file.path_key;
    syscall->resolver.dentry = syscall->open.dentry;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_OPEN : 0;
    syscall->resolver.callback = dr_type == DR_KPROBE ? DR_OPEN_CALLBACK_KPROBE_KEY : DR_OPEN_CALLBACK_TRACEPOINT_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    // tail call
    resolve_dentry(ctx, dr_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_OPEN);
    return 0;
}


int __attribute__((always_inline)) kprobe_sys_open_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_open_ret(ctx, retval, DR_KPROBE);
}

SYSCALL_KRETPROBE(creat) {
    return kprobe_sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(open_by_handle_at) {
    return kprobe_sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(truncate) {
    return kprobe_sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(open) {
    return kprobe_sys_open_ret(ctx);
}

SYSCALL_COMPAT_KRETPROBE(openat) {
    return kprobe_sys_open_ret(ctx);
}

SYSCALL_KRETPROBE(openat2) {
    return kprobe_sys_open_ret(ctx);
}

SEC("tracepoint/handle_sys_open_exit")
int tracepoint_handle_sys_open_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_open_ret(args, args->ret, DR_TRACEPOINT);
}

SEC("kretprobe/io_openat2")
int kretprobe_io_openat2(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_open_ret(ctx, retval, DR_KPROBE);
}

SEC("kprobe/filp_close")
int kprobe_filp_close(struct pt_regs *ctx) {
    struct file *file = (struct file *) PT_REGS_PARM1(ctx);
    u32 mount_id = get_file_mount_id(file);
    if (mount_id) {
        dec_mount_ref(ctx, mount_id);
    }

    return 0;
}

int __attribute__((always_inline)) dr_open_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_OPEN);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_OPEN);
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_INVALID) {
        return 0;
    }

    struct open_event_t event = {
        .syscall.retval = retval,
        .event.async = syscall->async,
        .event.saved_by_ad = syscall->resolver.saved_by_ad,
        .event.is_activity_dump_sample = syscall->resolver.ad_state == ACTIVITY_DUMP_RUNNING,
        .file = syscall->open.file,
        .flags = syscall->open.flags,
        .mode = syscall->open.mode,
    };

    fill_file_metadata(syscall->open.dentry, &event.file.metadata);
    struct proc_cache_t *entry;
    if (syscall->open.pid_tgid != 0) {
        entry = fill_process_context_with_pid_tgid(&event.process, syscall->open.pid_tgid);
    } else {
        entry = fill_process_context(&event.process);
    }
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_OPEN, event);
    return 0;
}

SEC("kprobe/dr_open_callback")
int __attribute__((always_inline)) kprobe_dr_open_callback(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return dr_open_callback(ctx, retval);
}

SEC("tracepoint/dr_open_callback")
int __attribute__((always_inline)) tracepoint_dr_open_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_open_callback(args, args->ret);
}

#endif
