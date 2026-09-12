#ifndef _HOOKS_UMOUNT_H_
#define _HOOKS_UMOUNT_H_

#include "constants/syscall_macro.h"
#include "constants/offsets/filesystem.h"
#include "helpers/filesystem.h"
#include "helpers/span_fill.h"
#include "helpers/syscalls.h"

HOOK_ENTRY("security_sb_umount")
int hook_security_sb_umount(ctx_t *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_UMOUNT,
        .umount = {
            .vfs = (struct vfsmount *)CTX_PARM1(ctx),
        }
    };

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

int __attribute__((always_inline)) sys_umount_ret_impl(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_UMOUNT);
    if (!syscall) {
        return 0;
    }

    if (retval) {
        return 0;
    }

    int mount_id = get_vfsmount_mount_id(syscall->umount.vfs);

    struct umount_event_t *event = SPAN_FILL_EVENT(struct umount_event_t, EVENT_UMOUNT);
    if (!event) {
        return 0;
    }
    event->syscall.retval = retval;
    event->mount_id = mount_id;

    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);

    span_fill_tail_call(ctx, prog_type);

    return 0;
}

int __attribute__((always_inline)) sys_umount_ret(void *ctx, int retval) {
    return sys_umount_ret_impl(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

HOOK_SYSCALL_EXIT(umount) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_umount_ret(ctx, retval);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_umount_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_umount_ret_impl(args, args->ret, TRACEPOINT_TYPE);
}

HOOK_ENTRY("cleanup_mnt")
int hook_cleanup_mnt(ctx_t *ctx) {
    struct mount *mnt = (struct mount *)CTX_PARM1(ctx);

    struct mount_released_event_t event = {};
    event.mount_id = get_mount_mount_id(mnt);
    event.mount_id_unique = get_mount_mount_id_unique(mnt);

    bump_mount_discarder_revision(event.mount_id);
    bump_high_path_id(event.mount_id);

    send_event(ctx, EVENT_MOUNT_RELEASED, event);

    return 0;
}

#endif
