#ifndef _HOOKS_UMOUNT_H_
#define _HOOKS_UMOUNT_H_

#include "constants/syscall_macro.h"
#include "constants/offsets/filesystem.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

HOOK_ENTRY("security_sb_umount")
int hook_security_sb_umount(ctx_t *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_UMOUNT,
        .umount = {
            .vfs = (struct vfsmount *)CTX_PARM1(ctx),
        }
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_umount_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_UMOUNT);
    if (!syscall) {
        return 0;
    }

    if (retval) {
        return 0;
    }

    int mount_id = get_vfsmount_mount_id(syscall->umount.vfs);

    struct umount_event_t event = {
        .syscall.retval = retval,
        .mount_id = mount_id
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_UMOUNT, event);

    umounted(ctx, mount_id);

    return 0;
}

HOOK_SYSCALL_EXIT(umount) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_umount_ret(ctx, retval);
}

SEC("tracepoint/handle_sys_umount_exit")
int tracepoint_handle_sys_umount_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_umount_ret(args, args->ret);
}

#endif
