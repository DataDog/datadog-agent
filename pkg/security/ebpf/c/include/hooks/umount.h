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

    // [do not merge] umount flakiness debug: snapshot the mount id while the mount is
    // guaranteed alive, to compare with what the exit hook reads.
    syscall.umount.dbg_entry_ts = bpf_ktime_get_ns();
    syscall.umount.dbg_owner = bpf_get_current_pid_tgid();
    syscall.umount.dbg_entry_read_ret = dbg_read_vfsmount_mount_id(syscall.umount.vfs, &syscall.umount.dbg_entry_mount_id);

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

static __always_inline int sys_umount_ret_impl(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UMOUNT);
    if (!syscall) {
        return 0;
    }

    if (retval) {
        pop_syscall(EVENT_UMOUNT);
        return 0;
    }

    int mount_id = get_vfsmount_mount_id(syscall->umount.vfs);

    // [do not merge] umount flakiness debug
    struct vfsmount *dbg_vfs = syscall->umount.vfs;
    u64 dbg_entry_ts = syscall->umount.dbg_entry_ts;
    u32 dbg_entry_mount_id = syscall->umount.dbg_entry_mount_id;
    s32 dbg_entry_read_ret = syscall->umount.dbg_entry_read_ret;
    u32 dbg_reread = 0;
    s32 dbg_exit_read_ret = dbg_read_vfsmount_mount_id(dbg_vfs, &dbg_reread);
    u32 dbg_mnt_flags = 0;
    bpf_probe_read(&dbg_mnt_flags, sizeof(dbg_mnt_flags), (char *)dbg_vfs + 16); // offsetof(struct vfsmount, mnt_flags)
    // re-read the cache entry after the mount id reads: if another task claimed the
    // element in between, owner/type/vfs no longer match what was read above.
    u64 dbg_owner_after = syscall->umount.dbg_owner;
    u64 dbg_type_after = syscall->type;
    u64 dbg_vfs_after = (u64)syscall->umount.vfs;

    pop_syscall(EVENT_UMOUNT);

    struct umount_event_t *event = SPAN_FILL_EVENT(struct umount_event_t, EVENT_UMOUNT);
    if (!event) {
        return 0;
    }
    event->syscall.retval = retval;
    event->mount_id = mount_id;
    event->dbg_entry_mount_id = dbg_entry_mount_id;
    event->dbg_entry_read_ret = dbg_entry_read_ret;
    event->dbg_exit_read_ret = dbg_exit_read_ret;
    event->dbg_vfs = (u64)dbg_vfs;
    event->dbg_entry_ts = dbg_entry_ts;
    event->dbg_exit_ts = bpf_ktime_get_ns();
    event->dbg_prog_type = prog_type;
    event->dbg_tid = (u32)bpf_get_current_pid_tgid();
    event->dbg_exit_mnt_flags = dbg_mnt_flags;
    event->dbg_exit_mount_id_reread = dbg_reread;
    event->dbg_owner_after = dbg_owner_after;
    event->dbg_type_after = dbg_type_after;
    event->dbg_vfs_after = dbg_vfs_after;

    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);

    span_fill_tail_call(ctx, prog_type);

    return 0;
}

static __always_inline int sys_umount_ret(void *ctx, int retval) {
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
    event.dbg_tid = (u32)bpf_get_current_pid_tgid(); // [do not merge] umount flakiness debug

    bump_mount_discarder_revision(event.mount_id);
    bump_high_path_id(event.mount_id);

    send_event(ctx, EVENT_MOUNT_RELEASED, event);

    return 0;
}

#endif
