#ifndef _HOOKS_MPROTECT_H_
#define _HOOKS_MPROTECT_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY0(mprotect) {
    struct policy_t policy = fetch_policy(EVENT_MPROTECT);
    if (is_discarded_by_process(policy.mode, EVENT_MPROTECT)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_MPROTECT,
    };

    cache_syscall(&syscall);
    return 0;
}

HOOK_ENTRY("security_file_mprotect")
int hook_security_file_mprotect(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MPROTECT);
    if (!syscall) {
        return 0;
    }

    u64 flags_offset;
    LOAD_CONSTANT("vm_area_struct_flags_offset", flags_offset);

    // Retrieve vma information
    struct vm_area_struct *vma = (struct vm_area_struct *)CTX_PARM1(ctx);
    bpf_probe_read(&syscall->mprotect.vm_protection, sizeof(syscall->mprotect.vm_protection), (char*)vma + flags_offset);
    bpf_probe_read(&syscall->mprotect.vm_start, sizeof(syscall->mprotect.vm_start), &vma->vm_start);
    bpf_probe_read(&syscall->mprotect.vm_end, sizeof(syscall->mprotect.vm_end), &vma->vm_end);
    syscall->mprotect.req_protection = (u64)CTX_PARM2(ctx);
    return 0;
}

int __attribute__((always_inline)) sys_mprotect_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MPROTECT);
    if (!syscall) {
        return 0;
    }

    if (filter_syscall(syscall, mprotect_approvers)) {
        return 0;
    }

    struct mprotect_event_t event = {
        .vm_protection = syscall->mprotect.vm_protection,
        .req_protection = syscall->mprotect.req_protection,
        .vm_start = syscall->mprotect.vm_start,
        .vm_end = syscall->mprotect.vm_end,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MPROTECT, event);
    return 0;
}

HOOK_SYSCALL_EXIT(mprotect) {
    return sys_mprotect_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

SEC("tracepoint/handle_sys_mprotect_exit")
int tracepoint_handle_sys_mprotect_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mprotect_ret(args, args->ret);
}

#endif
