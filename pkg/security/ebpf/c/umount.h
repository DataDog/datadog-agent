#ifndef _UMOUNT_H_
#define _UMOUNT_H_

#include "syscalls.h"

struct umount_event_t {
    struct event_t event;
    struct process_data_t process;
    char container_id[CONTAINER_ID_LEN];
    int mount_id;

};

SEC("kprobe/security_sb_umount")
int kprobe__security_sb_umount(struct pt_regs *ctx) {
    struct syscall_cache_t syscall = {
        .umount = {
            .vfs = (struct vfsmount *)PT_REGS_PARM1(ctx),
        }
    };

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KRETPROBE(umount) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct umount_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_UMOUNT,
        .event.timestamp = bpf_ktime_get_ns(),
        .mount_id = get_vfsmount_mount_id(syscall->umount.vfs),
    };

    fill_process_data(&event.process);

    // add process cache data
    struct proc_cache_t *entry = get_pid_cache(syscall->pid);
    if (entry) {
        copy_container_id(event.container_id, entry->container_id);
        event.process.numlower = entry->numlower;
    }

    send_mountpoints_events(ctx, event);

    return 0;
}

#endif
