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

long __attribute__((always_inline)) trace__sys_prctl(void *ctx, u8 async, int option, void *arg2, const char *arg5) {
    // Unrelated to the prctl event, and ahead of everything it needs: a process
    // naming an anonymous mapping OTEL_CTX is publishing its OTel process context.
    handle_otel_process_ctx_naming(ctx, option, (unsigned long)arg2, arg5);

    // Early return if the probe was attach for the process context notification.
    if (!is_event_enabled(EVENT_PRCTL)) {
        return 0;
    }

    if (is_discarded_by_pid()) {
        return 0;
    }
    struct policy_t policy = fetch_policy(EVENT_PRCTL);
    struct syscall_cache_t syscall = {
        .type = EVENT_PRCTL,
        .policy = policy,
        .prctl = {
            .option = option,
        }
    };

    if (approve_syscall(&syscall, prctl_approvers) == DISCARDED) {
        return 0;
    }

    if(option == PR_SET_NAME) {
        int n = bpf_probe_read_str(&syscall.prctl.name, MAX_PRCTL_NAME_LEN + 1, arg2);
        syscall.prctl.name_size_to_send = n;
        if (n > MAX_PRCTL_NAME_LEN) {
            syscall.prctl.name_truncated = 1;
        } else if (n < 0) {
            syscall.prctl.name_size_to_send = 0;
        }

        syscall.prctl.name[15] = 0;
        clean_str_trailing_zeros(syscall.prctl.name, MAX_PRCTL_NAME_LEN, MAX_PRCTL_NAME_LEN + 1);
        if (is_prctl_pr_name_discarder(syscall.prctl.name)) {
            return 0;
        };
    }

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

int __attribute__((always_inline)) sys_prctl_ret_impl(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_PRCTL);
    if (!syscall) {
        return 0;
    }

    struct prctl_event_t *event = SPAN_FILL_EVENT(struct prctl_event_t, EVENT_PRCTL);
    if (!event) {
        return 0;
    }
    event->syscall.retval = retval;
    event->event.flags = syscall->async;
    event->option = syscall->prctl.option;
    event->name_truncated = syscall->prctl.name_truncated;
    bpf_probe_read_str(&event->name, MAX_PRCTL_NAME_LEN, &syscall->prctl.name);
    event->sent_size = (syscall->prctl.name_size_to_send >= MAX_PRCTL_NAME_LEN)
        ? MAX_PRCTL_NAME_LEN
        : syscall->prctl.name_size_to_send;
    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);
    span_fill_tail_call(ctx, prog_type);
    return 0;
}

int __attribute__((always_inline)) sys_prctl_ret(void *ctx, int retval) {
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
