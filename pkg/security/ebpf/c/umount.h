#ifndef _UMOUNT_H_
#define _UMOUNT_H_

#include "syscalls.h"

struct umount_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    int mount_id;

};

SYSCALL_KPROBE0(umount) {
    return 0;
}

SEC("kprobe/security_sb_umount")
int kprobe__security_sb_umount(struct pt_regs *ctx) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_UMOUNT,
        .umount = {
            .vfs = (struct vfsmount *)PT_REGS_PARM1(ctx),
        }
    };

    cache_syscall(&syscall, EVENT_UMOUNT);
    return 0;
}

SYSCALL_KRETPROBE(umount) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_UMOUNT);
    if (!syscall)
        return 0;

    struct umount_event_t event = {
        .event.type = EVENT_UMOUNT,
        .event.timestamp = bpf_ktime_get_ns(),
        .syscall .retval = PT_REGS_RC(ctx),
        .mount_id = get_vfsmount_mount_id(syscall->umount.vfs),
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    send_mountpoints_events(ctx, event);

    return 0;
}

#endif
