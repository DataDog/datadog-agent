#ifndef _MOUNT_H_
#define _MOUNT_H_

#include "syscalls.h"

#define FSTYPE_LEN 16

struct mount_event_t {
    struct event_t event;
    struct process_data_t process;
    int new_mount_id;
    int new_group_id;
    dev_t new_device;
    int parent_mount_id;
    unsigned long parent_ino;
    char fstype[FSTYPE_LEN];
};

SYSCALL_KPROBE(mount) {
    struct syscall_cache_t syscall = {};
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) ctx->di;
    bpf_probe_read(&syscall.mount.fstype, sizeof(void *), &PT_REGS_PARM3(ctx));
#else
    syscall.mount.fstype = (void *)PT_REGS_PARM3(ctx);
#endif
    cache_syscall(&syscall);
    return 0;
}

SEC("kprobe/attach_recursive_mnt")
int kprobe__attach_recursive_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    syscall->mount.src_mnt = (struct mount *)PT_REGS_PARM1(ctx);
    syscall->mount.dest_mnt = (struct mount *)PT_REGS_PARM2(ctx);
    syscall->mount.dest_mountpoint = (struct mountpoint *)PT_REGS_PARM3(ctx);

    cache_syscall(syscall);
    return 0;
}

SEC("kprobe/propagate_mnt")
int kprobe__propagate_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    syscall->mount.dest_mnt = (struct mount *)PT_REGS_PARM1(ctx);
    syscall->mount.dest_mountpoint = (struct mountpoint *)PT_REGS_PARM2(ctx);
    syscall->mount.src_mnt = (struct mount *)PT_REGS_PARM3(ctx);

    cache_syscall(syscall);
    return 0;
}

SYSCALL_KRETPROBE(mount) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct dentry *dentry = get_mountpoint_dentry(syscall->mount.dest_mountpoint);
    struct path_key_t path_key = {
        .mount_id = get_mount_mount_id(syscall->mount.dest_mnt),
        .ino = get_dentry_ino(dentry),
    };
    struct mount_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_MOUNT,
        .event.timestamp = bpf_ktime_get_ns(),
        .new_mount_id = get_mount_mount_id(syscall->mount.src_mnt),
        .new_group_id = get_mount_peer_group_id(syscall->mount.src_mnt),
        .new_device = get_mount_dev(syscall->mount.src_mnt),
        .parent_mount_id = path_key.mount_id,
        .parent_ino = path_key.ino,
    };
    bpf_probe_read_str(&event.fstype, FSTYPE_LEN, syscall->mount.fstype);

    fill_process_data(&event.process);
    resolve_dentry(dentry, path_key);

    send_event(ctx, event);

    return 0;
}

#endif
