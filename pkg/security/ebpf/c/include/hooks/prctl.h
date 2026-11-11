#ifndef _HOOKS_PRCTL_H_
#define _HOOKS_PRCTL_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/process.h"
#include "helpers/span_fill.h"
#include "helpers/span_otel.h"
#include "helpers/syscalls.h"
#include "helpers/strings.h"
#include <linux/prctl.h>

static __always_inline long approve_prctl_event(struct syscall_cache_t *syscall, int option, void *arg2) {
    if (!is_event_enabled(EVENT_PRCTL)) {
        return 0;
    }

    if (is_discarded_by_pid()) {
        return 0;
    }

    syscall->policy = fetch_policy(EVENT_PRCTL);
    if (approve_syscall(syscall, prctl_approvers) == DISCARDED) {
        return 0;
    }

    if (option == PR_SET_NAME) {
        int n = bpf_probe_read_str(&syscall->prctl.name, MAX_PRCTL_NAME_LEN + 1, arg2);
        syscall->prctl.name_size_to_send = n;
        if (n > MAX_PRCTL_NAME_LEN) {
            syscall->prctl.name_truncated = 1;
        } else if (n < 0) {
            syscall->prctl.name_size_to_send = 0;
        }

        syscall->prctl.name[15] = 0;
        clean_str_trailing_zeros(syscall->prctl.name, MAX_PRCTL_NAME_LEN, MAX_PRCTL_NAME_LEN + 1);
        if (is_prctl_pr_name_discarder(syscall->prctl.name)) {
            return 0;
        };
    }

    return 1;
}

long __attribute__((always_inline)) trace__sys_prctl(void *ctx, u8 async, int option, void *arg2, const char *arg5) {
    struct syscall_cache_t syscall = {
        .type = EVENT_PRCTL,
        .prctl = {
            .option = option,
        }
    };

    if (is_otel_process_ctx_naming(option, (unsigned long)arg2, arg5)) {
        syscall.prctl.flags |= PRCTL_FLAG_OTEL_PROCESS_CTX;
    }

    if (approve_prctl_event(&syscall, option, arg2)) {
        syscall.prctl.flags |= PRCTL_FLAG_SEND_EVENT;
    }

    if (!syscall.prctl.flags) {
        return 0;
    }

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

static __always_inline int sys_prctl_ret_impl(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_PRCTL);
    if (!syscall) {
        return 0;
    }

    // before the prctl event, which ends with a tail call
    if (syscall->prctl.flags & PRCTL_FLAG_OTEL_PROCESS_CTX) {
        send_otel_process_ctx_event(ctx);
    }

    if (!(syscall->prctl.flags & PRCTL_FLAG_SEND_EVENT)) {
        goto pop_and_exit;
    }

    struct prctl_event_t *event = SPAN_FILL_EVENT(struct prctl_event_t, EVENT_PRCTL);
    if (!event) {
        goto pop_and_exit;
    }
    event->syscall.retval = retval;
    event->event.flags = syscall->async;
    event->option = syscall->prctl.option;
    event->name_truncated = syscall->prctl.name_truncated;
    bpf_probe_read_str(&event->name, MAX_PRCTL_NAME_LEN, &syscall->prctl.name);
    event->sent_size = (syscall->prctl.name_size_to_send >= MAX_PRCTL_NAME_LEN)
        ? MAX_PRCTL_NAME_LEN
        : syscall->prctl.name_size_to_send;
    pop_syscall(EVENT_PRCTL);

    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);
    span_fill_tail_call(ctx, prog_type);

pop_and_exit:
    pop_syscall(EVENT_PRCTL);
    return 0;
}

static __always_inline int sys_prctl_ret(void *ctx, int retval) {
    return sys_prctl_ret_impl(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

// arg5 is the name of the mapping
HOOK_SYSCALL_ENTRY5(prctl, int, option, void *, arg2, unsigned long, arg3, unsigned long, arg4, const char *, arg5) {
    return trace__sys_prctl(ctx, SYNC_SYSCALL, option, arg2, arg5);
}

HOOK_SYSCALL_EXIT(prctl) {
    int retval = SYSCALL_PARMRET(ctx);

    return sys_prctl_ret(ctx, retval);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_prctl_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_prctl_ret_impl(args, args->ret, TRACEPOINT_TYPE);
}

#endif
