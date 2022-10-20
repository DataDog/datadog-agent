#ifndef _MOUNT_H_
#define _MOUNT_H_

#include "syscalls.h"

#define FSTYPE_LEN 16

struct mount_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    u32 mount_id;
    u32 group_id;
    dev_t device;
    u32 parent_mount_id;
    unsigned long parent_inode;
    unsigned long root_inode;
    u32 root_mount_id;
    u32 padding;
    char fstype[FSTYPE_LEN];
};

SYSCALL_COMPAT_KPROBE3(mount, const char*, source, const char*, target, const char*, fstype) {
    struct syscall_cache_t syscall = {
        .type = EVENT_MOUNT,
    };

    cache_syscall(&syscall);
    return 0;
}

SEC("kprobe/attach_recursive_mnt")
int kprobe_attach_recursive_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    syscall->mount.src_mnt = (struct mount *)PT_REGS_PARM1(ctx);
    syscall->mount.dest_mnt = (struct mount *)PT_REGS_PARM2(ctx);
    syscall->mount.dest_mountpoint = (struct mountpoint *)PT_REGS_PARM3(ctx);

    // resolve root dentry
    struct dentry *dentry = get_vfsmount_dentry(get_mount_vfsmount(syscall->mount.src_mnt));
    syscall->mount.root_key.mount_id = get_mount_mount_id(syscall->mount.src_mnt);
    syscall->mount.root_key.ino = get_dentry_ino(dentry);

    struct super_block *sb = get_dentry_sb(dentry);
    struct file_system_type *s_type = get_super_block_fs(sb);
    bpf_probe_read(&syscall->mount.fstype, sizeof(syscall->mount.fstype), &s_type->name);

    syscall->resolver.key = syscall->mount.root_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/propagate_mnt")
int kprobe_propagate_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    syscall->mount.dest_mnt = (struct mount *)PT_REGS_PARM1(ctx);
    syscall->mount.dest_mountpoint = (struct mountpoint *)PT_REGS_PARM2(ctx);
    syscall->mount.src_mnt = (struct mount *)PT_REGS_PARM3(ctx);

    // resolve root dentry
    struct dentry *dentry = get_vfsmount_dentry(get_mount_vfsmount(syscall->mount.src_mnt));
    syscall->mount.root_key.mount_id = get_mount_mount_id(syscall->mount.src_mnt);
    syscall->mount.root_key.ino = get_dentry_ino(dentry);

    struct super_block *sb = get_dentry_sb(dentry);
    struct file_system_type *s_type = get_super_block_fs(sb);
    bpf_probe_read(&syscall->mount.fstype, sizeof(syscall->mount.fstype), &s_type->name);

    syscall->resolver.key = syscall->mount.root_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

int __attribute__((always_inline)) sys_mount_ret(void *ctx, int retval, int dr_type) {
    if (retval) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    struct dentry *dentry = get_mountpoint_dentry(syscall->mount.dest_mountpoint);
    struct path_key_t path_key = {
        .mount_id = get_mount_mount_id(syscall->mount.dest_mnt),
        .ino = get_dentry_ino(dentry),
    };
    syscall->mount.path_key = path_key;

    syscall->resolver.key = path_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = dr_type == DR_KPROBE ? DR_MOUNT_CALLBACK_KPROBE_KEY : DR_MOUNT_CALLBACK_TRACEPOINT_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, dr_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_MOUNT);
    return 0;
}

SYSCALL_COMPAT_KRETPROBE(mount) {
    int retval = PT_REGS_RC(ctx);
    return sys_mount_ret(ctx, retval, DR_KPROBE);
}

SEC("tracepoint/handle_sys_mount_exit")
int tracepoint_handle_sys_mount_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mount_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_mount_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    struct mount_event_t event = {
        .syscall.retval = retval,
        .event.async = 0,
        .mount_id = get_mount_mount_id(syscall->mount.src_mnt),
        .group_id = get_mount_peer_group_id(syscall->mount.src_mnt),
        .device = get_mount_dev(syscall->mount.src_mnt),
        .parent_mount_id = syscall->mount.path_key.mount_id,
        .parent_inode = syscall->mount.path_key.ino,
        .root_inode = syscall->mount.root_key.ino,
        .root_mount_id = syscall->mount.root_key.mount_id,
    };
    bpf_probe_read_str(&event.fstype, FSTYPE_LEN, (void*) syscall->mount.fstype);

    if (event.mount_id == 0 && event.device == 0) {
        return 0;
    }

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MOUNT, event);

    return 0;
}

SEC("kprobe/dr_mount_callback")
int __attribute__((always_inline)) kprobe_dr_mount_callback(struct pt_regs *ctx) {
    int ret = PT_REGS_RC(ctx);
    return dr_mount_callback(ctx, ret);
}

SEC("tracepoint/dr_mount_callback")
int __attribute__((always_inline)) tracepoint_dr_mount_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_mount_callback(args, args->ret);
}

#endif
