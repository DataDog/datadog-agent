#ifndef _HOOKS_SETSID_H_
#define _HOOKS_SETSID_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY0(setsid) {
    return 0;
}

HOOK_SYSCALL_EXIT(setsid) {
    int retval = (int)SYSCALL_PARMRET(ctx);
    if (retval < 0) {
        return 0;
    }

    // After a successful setsid(2), the calling task is the new session leader,
    // so its session ID equals its (host-namespace) PID by definition.
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry) {
        return 0;
    }

    pid_entry->sid = pid;
    return 0;
}

#endif // _HOOKS_SETSID_H_
