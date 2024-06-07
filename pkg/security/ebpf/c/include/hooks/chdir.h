#ifndef _HOOKS_CHDIR_H_
#define _HOOKS_CHDIR_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

long __attribute__((always_inline)) trace__sys_chdir(const char *path) {
    struct policy_t policy = fetch_policy(EVENT_CHDIR);
    if (is_discarded_by_process(policy.mode, EVENT_CHDIR)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_CHDIR,
        .policy = policy,
        .chdir = {}
    };

    collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0), (void *)path, NULL, NULL);
    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY1(chdir, const char*, path)
{
    return trace__sys_chdir(path);
}

HOOK_SYSCALL_ENTRY1(fchdir, unsigned int, fd)
{
    return trace__sys_chdir(NULL);
}

HOOK_ENTRY("set_fs_pwd")
int hook_set_fs_pwd(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_CHDIR);
    if (!syscall) {
        return 0;
    }

    if (syscall->chdir.dentry) {
        return 0;
    }

    struct path *path = (struct path *)CTX_PARM2(ctx);
    struct dentry *dentry = get_path_dentry(path);

    if (is_non_mountable_dentry(dentry)) {
        pop_syscall(EVENT_CHDIR);
        return 0;
    }

    syscall->chdir.dentry = dentry;
    syscall->chdir.file.path_key = get_dentry_key_path(syscall->chdir.dentry, path);

    set_file_inode(dentry, &syscall->chdir.file, 0);

    if (filter_syscall(syscall, chdir_approvers)) {
        return mark_as_discarded(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) sys_chdir_ret(void *ctx, int retval, int dr_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_CHDIR);
    if (!syscall) {
        return 0;
    }
    if (IS_UNHANDLED_ERROR(retval)) {
        discard_syscall(syscall);
        return 0;
    }

    set_file_inode(syscall->chdir.dentry, &syscall->chdir.file, 0);

    syscall->resolver.key = syscall->chdir.file.path_key;
    syscall->resolver.dentry = syscall->chdir.dentry;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_CHDIR : 0;
    syscall->resolver.callback = select_dr_key(dr_type, DR_CHDIR_CALLBACK_KPROBE_KEY, DR_CHDIR_CALLBACK_TRACEPOINT_KEY);
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;
    syscall->resolver.sysretval = retval;

    resolve_dentry(ctx, dr_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_CHDIR);
    return 0;
}

HOOK_SYSCALL_EXIT(chdir) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_chdir_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

HOOK_SYSCALL_EXIT(fchdir) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_chdir_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

SEC("tracepoint/handle_sys_chdir_exit")
int tracepoint_handle_sys_chdir_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_chdir_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_chdir_callback(void *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_CHDIR);
    if (!syscall) {
        return 0;
    }

    s64 retval = syscall->resolver.sysretval;

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_CHDIR);
        return 0;
    }

    struct chdir_event_t event = {
        .syscall.retval = retval,
        .syscall_ctx.id = syscall->ctx_id,
        .file = syscall->chdir.file,
    };

    fill_file(syscall->chdir.dentry, &event.file);
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_CHDIR, event);
    return 0;
}

TAIL_CALL_TARGET("dr_chdir_callback")
int tail_call_target_dr_chdir_callback(ctx_t *ctx) {
    return dr_chdir_callback(ctx);
}

SEC("tracepoint/dr_chdir_callback")
int tracepoint_dr_chdir_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_chdir_callback(args);
}

#endif
