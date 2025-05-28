#ifndef _HOOKS_SETRLIMIT_H_
#define _HOOKS_SETRLIMIT_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

struct setrlimit_syscall_t {
    int resource;
    struct rlimit *rlim;
};

int __attribute__((always_inline)) trace__sys_setrlimit(int resource, struct rlimit *rlim) {
    bpf_printk("======= sarra setrlimit\n");
    if (is_discarded_by_pid()) {
        return 0;
    }

    // Read the rlimit values from user space
    struct rlimit krlim = {};
    if (bpf_probe_read(&krlim, sizeof(krlim), rlim) != 0) {
        krlim.rlim_cur = 0;
        krlim.rlim_max = 0;
    }

    struct policy_t policy = fetch_policy(EVENT_SETRLIMIT);
    struct syscall_cache_t syscall = {
        .type = EVENT_SETRLIMIT,
        .policy = policy,
        .setrlimit = {
            .resource = resource,
            .rlim_cur = krlim.rlim_cur,
            .rlim_max = krlim.rlim_max,
            .pid = 0,
        }
    };

    cache_syscall(&syscall);
    return 0;
}



// //HOOK_ENTRY("do_prlimit"){
// int hook_do_prlimit(ctx_t *ctx) {
//     bpf_printk(" =============== sarra do_prlimit\n");
//     return 0;
// }



HOOK_SYSCALL_ENTRY2(setrlimit, int, resource, struct rlimit *, rlim) {
    bpf_printk("======= sarra setrlimit\n");
    return trace__sys_setrlimit(resource, rlim);
}
HOOK_SYSCALL_ENTRY3(prlimit64, pid_t, pid, int, resource, struct rlimit *, rlim) {
    bpf_printk("======= sarra prlimit\n");
    return trace__sys_setrlimit(resource, rlim);
}


int __attribute__((always_inline)) sys_setrlimit_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SETRLIMIT);
    if (!syscall) {
        return 0;
    }

    struct setrlimit_event_t event = {
        .event = {
            .timestamp = bpf_ktime_get_ns(),
            .type = EVENT_SETRLIMIT,
            .flags = 0,
        },
        .syscall = {
            .retval = retval,
        },
        .resource = syscall->setrlimit.resource,
        .rlim_cur = syscall->setrlimit.rlim_cur,
        .rlim_max = syscall->setrlimit.rlim_max,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SETRLIMIT, event);
    return 0;
}

HOOK_SYSCALL_EXIT(setrlimit) {
    int retval = SYSCALL_PARMRET(ctx);
    bpf_printk(" =============== sarra setrlimit exit\n");
    return sys_setrlimit_ret(ctx, retval);
}
HOOK_SYSCALL_EXIT(prlimit64) {
    int retval = SYSCALL_PARMRET(ctx);
    bpf_printk(" =============== sarra prlimit64 exit\n");
    return sys_setrlimit_ret(ctx, retval);
}

// TAIL_CALL_TRACEPOINT_FNC(handle_sys_setrlimit_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
//     return sys_setrlimit_ret(args, args->ret);
// }

#endif
