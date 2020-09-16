#ifndef _OPEN_H_
#define _OPEN_H_

#include "filters.h"
#include "syscalls.h"
#include "process.h"
#include "open_filter.h"

struct bpf_map_def SEC("maps/open_policy") open_policy = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct policy_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/open_basename_approvers") open_basename_approvers = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = BASENAME_FILTER_SIZE,
    .value_size = sizeof(struct filter_t),
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

struct bpf_map_def SEC("maps/open_flags_discarders") open_flags_discarders = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/open_process_inode_approvers") open_process_inode_approvers = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct filter_t),
    .max_entries = 256,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/open_path_inode_discarders") open_path_inode_discarders = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct path_key_t),
    .value_size = sizeof(struct filter_t),
    .max_entries = 512,
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
        .type = EVENT_OPEN,
        .policy = {.mode = ACCEPT},
        .open = {
            .flags = flags,
            .mode = mode,
        }
    };

    u32 key = 0;
    struct policy_t *policy = bpf_map_lookup_elem(&open_policy, &key);
    if (policy) {
        syscall.policy.mode = policy->mode;
        syscall.policy.flags = policy->flags;
    }

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(creat) {
    int flags = O_CREAT|O_WRONLY|O_TRUNC;
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM2(ctx));
#else
    mode = (umode_t) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_openat(flags, mode);
}

SYSCALL_KPROBE(open_by_handle_at) {
    int flags;
    umode_t mode = 0;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&flags, sizeof(flags), &PT_REGS_PARM3(ctx));
#else
    flags = (int) PT_REGS_PARM3(ctx);
#endif
    return trace__sys_openat(flags, mode);
}

SYSCALL_KPROBE(truncate) {
    int flags = O_CREAT|O_WRONLY|O_TRUNC;
    umode_t mode = 0;
    return trace__sys_openat(flags, mode);
}

SYSCALL_KPROBE(open) {
    int flags;
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&flags, sizeof(flags), &PT_REGS_PARM2(ctx));
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM3(ctx));
#else
    flags = (int) PT_REGS_PARM2(ctx);
    mode = (umode_t) PT_REGS_PARM3(ctx);
#endif
    return trace__sys_openat(flags, mode);
}

SYSCALL_KPROBE(openat) {
    int flags;
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&flags, sizeof(flags), &PT_REGS_PARM3(ctx));
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM4(ctx));
#else
    flags = (int) PT_REGS_PARM3(ctx);
    mode = (umode_t) PT_REGS_PARM4(ctx);
#endif
    return trace__sys_openat(flags, mode);
}

int __attribute__((always_inline)) approve_by_basename(struct syscall_cache_t *syscall) {
    struct open_basename_t basename = {};
    get_dentry_name(syscall->open.dentry, &basename, sizeof(basename));

    struct filter_t *filter = bpf_map_lookup_elem(&open_basename_approvers, &basename);
    if (filter) {
#ifdef DEBUG
        bpf_printk("kprobe/vfs_open basename %s approved\n", basename.value);
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
        bpf_printk("kprobe/vfs_open flags %d approved\n", syscall->open.flags);
#endif
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) discard_by_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&open_flags_discarders, &key);
    if (flags != NULL && (syscall->open.flags & *flags) > 0) {
#ifdef DEBUG
        bpf_printk("kprobe/vfs_open flags %d discarded\n", syscall->open.flags);
#endif
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_by_process_inode(struct syscall_cache_t *syscall) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct proc_cache_t *proc = get_pid_cache(tgid);
    if (!proc) {
        return 0;
    }
    u64 inode = proc->executable.inode;
    struct filter_t *filter = bpf_map_lookup_elem(&open_process_inode_approvers, &inode);
    if (filter) {
#ifdef DEBUG
        bpf_printk("kprobe/vfs_open pid %d with inode %d approved\n", tgid, inode);
#endif
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) vfs_handle_open_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    // NOTE(safchain) could be moved only if pass_to_userspace == 1
    syscall->open.dir = (struct path *)PT_REGS_PARM1(ctx);
    syscall->open.dentry = get_path_dentry(syscall->open.dir);
    syscall->open.path_key = get_key(syscall->open.dentry, syscall->open.dir);

    if (syscall->policy.mode == NO_FILTER)
        goto no_filter;

    char pass_to_userspace = syscall->policy.mode == ACCEPT ? 1 : 0;

    if (syscall->policy.mode == DENY) {
        if ((syscall->policy.flags & BASENAME) > 0) {
            pass_to_userspace = approve_by_basename(syscall);
        }

        if (!pass_to_userspace && ((syscall->policy.flags & PROCESS_INODE))) {
            pass_to_userspace = approve_by_process_inode(syscall);
        }

        if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
           pass_to_userspace = approve_by_flags(syscall);
        }
    } else {
        if (pass_to_userspace && ((syscall->policy.flags & FLAGS))) {
            pass_to_userspace = !discard_by_flags(syscall);
        }
    }

    if (!pass_to_userspace) {
        pop_syscall();
    }

no_filter:

    return 0;
}

SEC("kprobe/vfs_truncate")
int kprobe__vfs_truncate(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    if (syscall->type == EVENT_OPEN) {
        return vfs_handle_open_event(ctx, syscall);
    }

    return 0;
}

SEC("kprobe/vfs_open")
int kprobe__vfs_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    switch(syscall->type) {
        case EVENT_OPEN:
            return vfs_handle_open_event(ctx, syscall);
        case EVENT_EXEC:
            return vfs_handle_exec_event(ctx, syscall);
    }

    return 0;
}

int __attribute__((always_inline)) trace__sys_open_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct open_event_t event = {
        .event.type = EVENT_OPEN,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .file = {
            .inode = syscall->open.path_key.ino,
            .mount_id = syscall->open.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->open.dentry),
        },
        .flags = syscall->open.flags,
        .mode = syscall->open.mode,
    };

    int ret = 0;
    if (syscall->policy.mode == NO_FILTER) {
        ret = resolve_dentry(syscall->open.dentry, syscall->open.path_key, NULL);
    } else {
        ret = resolve_dentry(syscall->open.dentry, syscall->open.path_key, &open_path_inode_discarders);
    }
    if (ret < 0) {
        return 0;
    }

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(creat) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_KRETPROBE(open_by_handle_at) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_KRETPROBE(truncate) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_KRETPROBE(open) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_KRETPROBE(openat) {
    return trace__sys_open_ret(ctx);
}

#endif
