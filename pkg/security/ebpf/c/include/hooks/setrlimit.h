#ifndef _HOOKS_SETRLIMIT_H_
#define _HOOKS_SETRLIMIT_H_

#include "constants/syscall_macro.h"   
#include "helpers/discarders.h"        
#include "helpers/syscalls.h"          
#include "events_definition.h"        

#define SETRLIMIT_RATE_LIMITER  100     

static const int important_resources[] = {
    RLIMIT_CPU,
    RLIMIT_FSIZE,
    RLIMIT_NOFILE,
    RLIMIT_STACK,
    RLIMIT_NPROC,
    RLIMIT_CORE
};

static __always_inline int handle_setrlimit_common(unsigned int resource, const struct rlimit __user *new_rlim, u32 target_pid)
{
    bool is_important = false;
    for (int i = 0; i < ARRAY_SIZE(important_resources); i++) {
        if (resource == important_resources[i]) {
            is_important = true;
            break;
        }
    }
    if (!is_important &&
        !pid_rate_limiter_allow(SETRLIMIT_RATE_LIMITER, 1)) {
        return 0;
    }

    struct rlimit rlim;
    if (bpf_probe_read_user(&rlim, sizeof(rlim), new_rlim) < 0) {
        return 0;
    }

    struct syscall_cache_t cache = {
        .type        = EVENT_SETRLIMIT,
        .setrlimit = {
            .resource     = resource,
            .pid          = target_pid,
            .rlim_cur     = rlim.rlim_cur,
            .rlim_max     = rlim.rlim_max,
        }
    };

    cache_syscall(&cache);
    return 0;
}

HOOK_SYSCALL_ENTRY2(setrlimit,
                    unsigned int, resource,
                    const struct rlimit __user *, new_rlim)
{
    return handle_setrlimit_common(resource, new_rlim, 0);
}

HOOK_ENTRY("security_task_setrlimit")
int hook_security_task_setrlimit(ctx_t *ctx)
{
    struct task_struct *task = (struct task_struct *)CTX_PARM1(ctx);
    if (!task) {
        return 0;
    }

    struct syscall_cache_t *cache = peek_syscall(EVENT_SETRLIMIT);
    if (!cache) {
        return 0;
    }

    // Get the root namespace PID for the target process
    u32 root_pid = get_root_nr_from_task_struct(task);
    if (root_pid == 0) {
        return 0;
    }

    // Update the cache with the target process information
    cache->setrlimit.pid = root_pid;

    return 0;
}

static __always_inline int
sys_setrlimit_ret(void *ctx, int ret)
{
    struct syscall_cache_t *cache = pop_syscall(EVENT_SETRLIMIT);
    if (!cache) {
        return 0;
    }

    if (ret != 0 && ret != -EPERM) {
        return 0;
    }

    if (cache->setrlimit.pid == 0) {
       u32 fallback = bpf_get_current_pid_tgid() >> 32;
       cache->setrlimit.pid = fallback;
    }

    struct setrlimit_event_t evt = {
        .syscall.retval = ret,
        .resource = cache->setrlimit.resource,
        .rlim_cur = cache->setrlimit.rlim_cur,
        .rlim_max = cache->setrlimit.rlim_max,
        .target = cache->setrlimit.pid,
    };

    struct proc_cache_t *pc = fill_process_context(&evt.process);
    fill_container_context(pc, &evt.container);
    fill_span_context(&evt.span);

    send_event(ctx, EVENT_SETRLIMIT, evt);
    return 0;
}

HOOK_SYSCALL_EXIT(setrlimit) {
    return sys_setrlimit_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_setrlimit_exit,
                         struct tracepoint_raw_syscalls_sys_exit_t *args)
{
    return sys_setrlimit_ret(args, args->ret);
}

HOOK_SYSCALL_ENTRY4(prlimit64,
                    pid_t, pid,
                    int, resource,
                    const struct rlimit __user *, new_limit,
                    struct rlimit __user *, old_limit)
{
    if (new_limit == NULL) {
        return 0;
    }
    
    return handle_setrlimit_common(resource, new_limit, pid);
}

HOOK_SYSCALL_EXIT(prlimit64) {
    return sys_setrlimit_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

#endif  // _HOOKS_SETRLIMIT_H_
