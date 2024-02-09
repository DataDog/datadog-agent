#ifndef _HOOKS_SELINUX_H_
#define _HOOKS_SELINUX_H_

#include "constants/offsets/filesystem.h"
#include "helpers/filesystem.h"
#include "helpers/selinux.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) handle_selinux_event(void *ctx, struct file *file, const char *buf, size_t count, enum selinux_source_event_t source_event) {
    struct syscall_cache_t syscall = {
        .type = EVENT_SELINUX,
        .policy = fetch_policy(EVENT_SELINUX),
        .selinux = {
            .payload.bool_value = -1,
        },
    };

    struct dentry *dentry = get_file_dentry(file);
    syscall.selinux.dentry = dentry;
    syscall.selinux.file.path_key.mount_id = get_file_mount_id(file);

    if (count < SELINUX_WRITE_BUFFER_LEN) {
        int value = parse_buf_to_bool(buf);
        switch (source_event) {
        case SELINUX_BOOL_CHANGE_SOURCE_EVENT:
            syscall.selinux.event_kind = SELINUX_BOOL_CHANGE_EVENT_KIND;
            syscall.selinux.payload.bool_value = value;
            break;
        case SELINUX_BOOL_COMMIT_SOURCE_EVENT:
            syscall.selinux.event_kind = SELINUX_BOOL_COMMIT_EVENT_KIND;
            syscall.selinux.payload.bool_value = value;
            break;
        case SELINUX_ENFORCE_CHANGE_SOURCE_EVENT:
            syscall.selinux.event_kind = SELINUX_STATUS_CHANGE_EVENT_KIND;
            if (value >= 0) {
                u32 key = SELINUX_ENFORCE_STATUS_ENFORCE_KEY;
                bpf_map_update_elem(&selinux_enforce_status, &key, &value, BPF_ANY);
            }
            fill_selinux_status_payload(&syscall);
            break;
        case SELINUX_DISABLE_CHANGE_SOURCE_EVENT:
            syscall.selinux.event_kind = SELINUX_STATUS_CHANGE_EVENT_KIND;
            if (value >= 0) {
                u32 key = SELINUX_ENFORCE_STATUS_DISABLE_KEY;
                bpf_map_update_elem(&selinux_enforce_status, &key, &value, BPF_ANY);
            }
            fill_selinux_status_payload(&syscall);
            break;
        }
    }
    // otherwise let's keep the value = error state.

    fill_file(syscall.selinux.dentry, &syscall.selinux.file);
    set_file_inode(syscall.selinux.dentry, &syscall.selinux.file, 0);

    syscall.resolver.key = syscall.selinux.file.path_key;
    syscall.resolver.dentry = syscall.selinux.dentry;
    syscall.resolver.discarder_type = syscall.policy.mode != NO_FILTER ? EVENT_SELINUX : 0;
    syscall.resolver.callback = DR_SELINUX_CALLBACK_KPROBE_KEY;
    syscall.resolver.iteration = 0;
    syscall.resolver.ret = 0;

    cache_syscall(&syscall);

    // tail call
    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_SELINUX);

    return 0;
}

int __attribute__((always_inline)) dr_selinux_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SELINUX);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_SELINUX);
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_INVALID) {
        return 0;
    }

    struct selinux_event_t event = {};
    event.event_kind = syscall->selinux.event_kind;
    event.file = syscall->selinux.file;
    event.payload = syscall->selinux.payload;

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SELINUX, event);
    return 0;
}

TAIL_CALL_TARGET("dr_selinux_callback")
int tail_call_target_dr_selinux_callback(ctx_t *ctx) {
    // int retval = PT_REGS_RC(ctx);
    int retval = 0;
    return dr_selinux_callback(ctx, retval);
}

#define PROBE_SEL_WRITE_FUNC(func_name, source_event)                       \
    HOOK_ENTRY(#func_name)                                                  \
    int hook_##func_name(ctx_t *ctx) {                                      \
        struct file *file = (struct file *)CTX_PARM1(ctx);                  \
        const char *buf = (const char *)CTX_PARM2(ctx);                     \
        size_t count = (size_t)CTX_PARM3(ctx);                              \
        /* selinux only supports ppos = 0 */                                \
        return handle_selinux_event(ctx, file, buf, count, (source_event)); \
    }

PROBE_SEL_WRITE_FUNC(sel_write_disable, SELINUX_DISABLE_CHANGE_SOURCE_EVENT)
PROBE_SEL_WRITE_FUNC(sel_write_enforce, SELINUX_ENFORCE_CHANGE_SOURCE_EVENT)
PROBE_SEL_WRITE_FUNC(sel_write_bool, SELINUX_BOOL_CHANGE_SOURCE_EVENT)
PROBE_SEL_WRITE_FUNC(sel_commit_bools_write, SELINUX_BOOL_COMMIT_SOURCE_EVENT)

#endif
