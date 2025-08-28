#ifndef _HOOKS_PRCTL_H_
#define _HOOKS_PRCTL_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"
#include "helpers/process.h"

long __attribute__((always_inline)) trace__sys_prctl(u8 async, int option, void * arg2) {
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
    int key = 0;
    struct prctl_event_t *event = bpf_map_lookup_elem(&prctl_event, &key);
    if (!event) {
        return 0;
    }
    switch (option) {
    case PR_SET_NAME: {
        int n = bpf_probe_read_str(event->name, MAX_PRCTL_NAME_LEN, arg2);
        if (n > 0) {
            syscall.prctl.name_truncated = (n == MAX_PRCTL_NAME_LEN) ? 1 : 0;
            syscall.prctl.name_size_to_send = n - 1;    
        } else {
            syscall.prctl.name_size_to_send = 0;
        }
        break;
        }
    }
    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_prctl_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_PRCTL);
    if (!syscall) {
        return 0;
    }
    int key = 0;
    struct prctl_event_t *event = bpf_map_lookup_elem(&prctl_event,&key);

    if (!event) {
        return 0;  
    }
    event->syscall.retval = retval;
    event->event.flags = syscall->async;
    event->option = syscall->prctl.option;
    event->name_truncated = syscall->prctl.name_truncated;
    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);
    fill_span_context(&event->span);
    int size_to_sent = (syscall->prctl.name_size_to_send >= MAX_PRCTL_NAME_LEN)
        ? MAX_PRCTL_NAME_LEN
        : syscall->prctl.name_size_to_send;
    event->sent_size = size_to_sent;
    send_event_with_size_ptr(ctx, EVENT_PRCTL, event, (offsetof(struct prctl_event_t, name) + size_to_sent));
    return 0;
}

HOOK_SYSCALL_ENTRY2(prctl, int, option, void *, arg2) {
    return trace__sys_prctl(SYNC_SYSCALL, option, arg2);
}

HOOK_SYSCALL_EXIT(prctl) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_prctl_ret(ctx, retval);
}
#endif
