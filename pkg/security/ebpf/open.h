#ifndef _OPEN_H_
#define _OPEN_H_

#include "syscalls.h"
#include "process.h"

struct bpf_map_def SEC("maps/open_policy") open_policy = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u8),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/open_basename_approvers") open_basename_approvers = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(32),
    .value_size = sizeof(u8),
    .max_entries = 256,
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
    if (filter_process())
        return 0;

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
#ifdef CONFIG_ARCH_HAS_SYSCALL_WRAPPER
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
#ifdef CONFIG_ARCH_HAS_SYSCALL_WRAPPER
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
    printk("kprobe/vfs_open\n");

    struct path *path = (struct path *)PT_REGS_PARM1(ctx);

    u32 key = 0;
    u8 *policy = bpf_map_lookup_elem(&open_policy, &key);
    if (policy != NULL && *policy > 0) {


        struct dentry *dentry;
        bpf_probe_read(&dentry, sizeof(dentry), &path->dentry);

        struct qstr qstr;
        bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);

        char basename[32];
        bpf_probe_read_str(basename, sizeof(basename), (void *)qstr.name);

        printk("kprobe/vfs_open check approver for: %s\n", basename);
        void *found = bpf_map_lookup_elem(&open_basename_approvers, &basename);
        if (found == NULL) {
            printk("kprobe/vfs_open reject: %s\n", basename);
            pop_syscall();
            return 0;
        } else {
            printk("kprobe/vfs_open approve: %s\n", basename);
        }
    }

    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

#ifdef DEBUG
    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), &path->dentry);

    struct qstr qstr;
    bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);

    printk("kprobe/vfs_open %s\n", qstr.name);
#endif

    syscall->open.dentry = get_path_dentry((struct path *)PT_REGS_PARM1(ctx));

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