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

struct bpf_map_def SEC("maps/open_basename_discarders") open_basename_discarders = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = BASENAME_FILTER_SIZE,
    .value_size = sizeof(struct filter_t),
    .max_entries = 256,
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
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct open_event_t {
    struct   event_t event;
    struct   process_data_t process;
    int           flags;
    int           mode;
    unsigned long inode;
    dev_t         dev;
    u32           padding;
};

int __attribute__((always_inline)) trace__sys_openat(int flags, umode_t mode) {
    struct syscall_cache_t syscall = {
        .open = {
            .flags = flags,
            .mode = mode
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(open) {
    int flags;
    umode_t mode;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
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
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&flags, sizeof(flags), &PT_REGS_PARM3(ctx));
    bpf_probe_read(&mode, sizeof(mode), &PT_REGS_PARM4(ctx));
#else
    flags = (int) PT_REGS_PARM3(ctx);
    mode = (umode_t) PT_REGS_PARM4(ctx);
#endif
    return trace__sys_openat(flags, mode);
}

SEC("kprobe/vfs_open")
int kprobe__vfs_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    // NOTE(safchain) could be move only if pass_to_userspace == 1
    syscall->open.dentry = get_path_dentry((struct path *)PT_REGS_PARM1(ctx));

    // array key index 0
    u32 ak0 = 0;

    struct policy_t zero = {.mode = ACCEPT};
    struct policy_t *policy = bpf_map_lookup_elem(&open_policy, &ak0);
    if (!policy) {
        policy = &zero;
    }

    struct open_basename_t basename = {};
    if ((policy->flags & BASENAME) > 0) {
        get_dentry_name(syscall->open.dentry, &basename, sizeof(basename));
    }

    char pass_to_userspace = policy->mode == ACCEPT ? 1 : 0;

    if (policy->mode == DENY) {
        if ((policy->flags & BASENAME) > 0) {
            struct filter_t *filter = bpf_map_lookup_elem(&open_basename_approvers, &basename);
            if (filter != NULL) {
                pass_to_userspace = 1;
#ifdef DEBUG
                printk("kprobe/vfs_open %s approved\n", basename.value);
#endif
            }
        }

        if (!pass_to_userspace && (policy->flags & FLAGS) > 0) {
            u32 *flags = bpf_map_lookup_elem(&open_flags_approvers, &ak0);
            if (flags != NULL && (syscall->open.flags & *flags) > 0) {
                pass_to_userspace = 1;
#ifdef DEBUG
                printk("kprobe/vfs_open %s approved by flags\n", basename.value);
#endif
            }
        }
    }

    // only check discarders if policy is ACCEPT
    if (policy->mode == ACCEPT) {
        if ((policy->flags & BASENAME) > 0) {
            struct filter_t *filter = bpf_map_lookup_elem(&open_basename_discarders, &basename);
            if (filter) {
                pass_to_userspace = 0;
#ifdef DEBUG
                printk("kprobe/vfs_open %s discarded\n", basename.value);
#endif
            }
        }

        if (pass_to_userspace) {
            u32 *flags = bpf_map_lookup_elem(&open_flags_discarders, &ak0);
            if (flags != NULL && (syscall->open.flags & *flags) > 0) {
                pass_to_userspace = 0;
#ifdef DEBUG
                printk("kprobe/vfs_open %s discarded by flags\n", basename.value);
#endif
            }
        }
    }

    if (!pass_to_userspace) {
        pop_syscall();
    }

    return 0;
}

int __attribute__((always_inline)) trace__sys_open_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct dentry *f_dentry = syscall->open.dentry;
    struct path_key_t path_key = get_dentry_key(f_dentry);

    struct open_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_MAY_OPEN,
        .event.timestamp = bpf_ktime_get_ns(),
        .flags = syscall->open.flags,
        .mode = syscall->open.mode,
        .dev = path_key.dev,
        .inode = path_key.ino,
    };

    fill_process_data(&event.process);
    resolve_dentry(f_dentry, path_key);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(open) {
    return trace__sys_open_ret(ctx);
}

SYSCALL_KRETPROBE(openat) {
    return trace__sys_open_ret(ctx);
}

#endif