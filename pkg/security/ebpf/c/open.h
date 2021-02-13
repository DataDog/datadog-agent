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
    if (discarded_by_process(policy.mode, EVENT_OPEN)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = SYSCALL_OPEN,
        .policy = policy,
        .open = {
            .flags = flags,
            .mode = mode,
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

int __attribute__((always_inline)) handle_open_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    if (syscall->open.path_key.ino) {
        return 0;
    }

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct inode *inode = (struct inode *)PT_REGS_PARM2(ctx);

    struct dentry *dentry = get_file_dentry(file);

    syscall->open.dentry = dentry;
    syscall->open.path_key = get_inode_key_path(inode, &file->f_path);

    set_path_key_inode(dentry, &syscall->open.path_key, 0);

    if (filter_syscall(syscall, open_approvers)) {
        return discard_syscall(syscall);
    }

    return 0;
}

SEC("kprobe/vfs_truncate")
int kprobe__vfs_truncate(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN);
    if (!syscall)
        return 0;

    if (syscall->open.dentry) {
        return 0;
    }

    struct path *path = (struct path *)PT_REGS_PARM1(ctx);

    struct dentry *dentry = get_path_dentry(path);

    syscall->open.dentry = dentry;
    syscall->open.path_key = get_dentry_key_path(syscall->open.dentry, path);

    set_path_key_inode(dentry, &syscall->open.path_key, 0);

    if (filter_syscall(syscall, open_approvers)) {
        return discard_syscall(syscall);
    }

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

    struct open_event_t event = {
        .syscall.retval = retval,
        .file = {
            .inode = syscall->open.path_key.ino,
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

SYSCALL_KRETPROBE(openat2) {
    return trace__sys_open_ret(ctx);
}

#endif
